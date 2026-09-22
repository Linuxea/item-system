package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
)

// UseRequest 使用请求。
type UseRequest struct {
	// Owner 使用者。
	Owner string
	// InstanceID 被使用的实例。
	InstanceID string
	// Count 使用次数，0 视为 1。宝箱连开 3 次即 Count=3，效果执行 3 轮。
	Count int64
	// Params 玩家输入的原始参数（通常是一段 JSON）。
	// 引擎不解析它，原样交给道具的 Bind 方法；不需要参数的道具传 nil。
	Params []byte
	// IdempotencyKey 幂等键，可选。
	IdempotencyKey string
}

// UseResult 使用结果。发生部分失败时，Consumed 反映真正生效的部分。
type UseResult struct {
	// InstanceID 被使用的实例。
	InstanceID string
	// Consumed 实际消耗数量。
	Consumed int64
	// Remaining 使用后剩余数量。
	Remaining int64
}

// Use 使用道具。
//
// 关键顺序（与"先执行后扣减"相反，这是刻意的）：
//
//	校验与参数绑定  ← 零副作用；参数缺失/非法在这里失败，道具分毫不动
//	CAS 预扣        ← 并发闸门：两个并发请求只有一个能扣成功，另一个拿到版本冲突
//	执行道具逻辑     ← 真正的副作用
//	失败则补偿回滚   ← 把没执行成功的份额加回去
//
// 若先执行后扣减，两个并发请求可以同时通过数量校验、各自执行一遍效果，
// 之后才有一个扣减失败 —— 道具被用了两次，只扣了一次。
func (e *Engine) Use(ctx context.Context, req UseRequest) (*UseResult, error) {
	times := req.Count
	if times <= 0 {
		times = 1
	}

	inst, err := e.mustOwn(ctx, req.Owner, req.InstanceID)
	if err != nil {
		return nil, err
	}
	if inst.Expired(e.now()) {
		return nil, item.ErrExpired
	}
	it, err := e.lookup(inst.ItemID)
	if err != nil {
		return nil, err
	}
	usable, ok := it.(item.Usable)
	if !ok {
		return nil, fmt.Errorf("%w: %s", item.ErrNotUsable, it.Name())
	}

	// 参数绑定：得到一个填好玩家输入的道具副本，原型不受影响。
	// 校验写在道具自己的 Bind 里，此刻尚未发生任何副作用。
	if p, ok := it.(item.Parameterized); ok {
		bound, err := p.Bind(req.Params)
		if err != nil {
			return nil, err
		}
		usable = bound
	}

	perUse := usable.ConsumePerUse()
	total := perUse * times
	if inst.Count < total {
		return nil, fmt.Errorf("%w: 需要 %d，持有 %d", item.ErrNotEnough, total, inst.Count)
	}

	if err := e.claim(ctx, req.IdempotencyKey); err != nil {
		return nil, err
	}

	// CAS 预扣。并发闸门就在这一步。
	expected := inst.Version
	inst.Count -= total
	inst.Version++
	if total > 0 {
		if err := e.store.Update(ctx, inst, expected); err != nil {
			e.release(ctx, req.IdempotencyKey)
			return nil, err
		}
	}

	// 执行。逐轮进行，记录真正跑完了几轮。
	var done int64
	var execErr error
	for i := int64(0); i < times; i++ {
		if err := usable.Use(ctx, req.Owner); err != nil {
			execErr = err
			break
		}
		done++
	}

	// 补偿：把没跑成的份额加回去。
	if undone := times - done; undone > 0 && perUse > 0 {
		refund := undone * perUse
		expected = inst.Version
		inst.Count += refund
		inst.Version++
		if err := e.store.Update(ctx, inst, expected); err != nil {
			// 回滚失败是需要人工介入的状态，如实上报，不掩盖原始错误。
			return nil, fmt.Errorf("使用失败且回滚失败（原始错误: %v）: %w", execErr, err)
		}
	}
	if done == 0 {
		e.release(ctx, req.IdempotencyKey)
		return nil, execErr
	}

	// 全部用完则删除实例。
	if inst.Count == 0 {
		if err := e.store.Delete(ctx, inst.ID, inst.Version); err != nil {
			return nil, err
		}
	}

	consumed := done * perUse
	e.publish(ctx, event.KindConsumed, inst, consumed, nil)
	res := &UseResult{InstanceID: inst.ID, Consumed: consumed, Remaining: inst.Count}
	return res, execErr
}

// LateGranter 延迟绑定的发放器。
//
// 它解开一个先有鸡还是先有蛋的问题：宝箱这类道具在构造时需要一个「发放器」，
// 而发放器就是引擎本身，引擎又需要先有全部道具才能构造。
// 做法是先造一个空壳交给道具，引擎构造完成后再 Bind 进去。
type LateGranter struct {
	mu     sync.RWMutex
	engine *Engine
	source string
	reason string
}

// NewLateGranter 创建延迟发放器；source/reason 会写进它发出的发放事件。
func NewLateGranter(source, reason string) *LateGranter {
	return &LateGranter{source: source, reason: reason}
}

// Bind 在引擎构造完成后注入引擎。必须在任何一次 Grant 之前调用。
func (g *LateGranter) Bind(e *Engine) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.engine = e
}

// Grant 发放道具，结构化满足 port.Granter。
func (g *LateGranter) Grant(ctx context.Context, owner, itemID string, count int64) error {
	g.mu.RLock()
	e := g.engine
	g.mu.RUnlock()
	if e == nil {
		return fmt.Errorf("engine: 发放器尚未绑定引擎")
	}
	_, err := e.Grant(ctx, GrantRequest{
		Owner: owner, ItemID: itemID, Count: count,
		Source: g.source, Reason: g.reason,
	})
	return err
}
