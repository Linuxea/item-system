package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
)

// Downgrade 一次降级的记录。
type Downgrade struct {
	// InstanceID 被降级的旧实例。
	InstanceID string
	// From 旧道具 ID。
	From string
	// To 新道具 ID。
	To string
	// NewInstanceID 降级后产生的新实例。
	NewInstanceID string
}

// ExpireResult 一次过期扫描的处置结果。
type ExpireResult struct {
	// Removed 被删除的实例 ID。
	Removed []string
	// Unequipped 被卸下但保留在背包的实例 ID。
	Unequipped []string
	// Downgraded 发生的降级。
	Downgraded []Downgrade
}

// RunExpiry 扫描并处置全部已到期的实例。
//
// 三种处置方式由道具自己声明（item.Expirable.OnExpire）：
//
//	ExpireRemove    卸下并删除
//	ExpireUnequip   仅卸下，实例留在背包
//	ExpireDowngrade 换成另一款道具（需同时实现 item.Downgradable）
//
// 降级目标若自身也有时效，则按目标自己的时效重新计时，由此形成
// vip3 -> vip1 -> vip0 这样的降级链：每一环都只知道自己的下一环。
//
// 生产环境把它挂在定时任务上；也可以用 Redis ZSet 按到期时刻调度后调用同一段逻辑。
func (e *Engine) RunExpiry(ctx context.Context) (*ExpireResult, error) {
	now := e.now()
	expired, err := e.store.ListExpiredBefore(ctx, now)
	if err != nil {
		return nil, err
	}
	res := &ExpireResult{}
	for _, inst := range expired {
		it, ok := e.items.Lookup(inst.ItemID)
		if !ok {
			continue
		}
		ex, ok := it.(item.Expirable)
		if !ok {
			continue // 道具已改成无时效，跳过，留给人工清理
		}
		if err := e.handleExpired(ctx, inst, it, ex.OnExpire(), res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// handleExpired 按策略处置一个已到期的实例。
func (e *Engine) handleExpired(ctx context.Context, inst *item.Instance, it item.Item, policy item.ExpirePolicy, res *ExpireResult) error {
	// 三种策略都先卸下。
	if inst.Equipped {
		if err := e.unequip(ctx, inst); err != nil {
			return err
		}
	}

	switch policy {
	case item.ExpireUnequip:
		expected := inst.Version
		inst.ExpiryHandled = true
		inst.Version++
		if err := e.store.Update(ctx, inst, expected); err != nil {
			return err
		}
		res.Unequipped = append(res.Unequipped, inst.ID)
		e.publish(ctx, event.KindExpired, inst, inst.Count, map[string]any{"policy": string(policy)})
		return nil

	case item.ExpireDowngrade:
		d, ok := it.(item.Downgradable)
		if !ok {
			return fmt.Errorf("%w: %s 声明了降级策略但未实现 Downgradable", item.ErrInvalidParam, it.ID())
		}
		return e.downgrade(ctx, inst, d.DowngradeTo(), res)

	default: // ExpireRemove
		if err := e.store.Delete(ctx, inst.ID, inst.Version); err != nil {
			return err
		}
		res.Removed = append(res.Removed, inst.ID)
		e.publish(ctx, event.KindExpired, inst, inst.Count, map[string]any{"policy": string(item.ExpireRemove)})
		return nil
	}
}

// downgrade 把实例换成目标道具的新实例：新实例按目标自己的时效重新计时。
func (e *Engine) downgrade(ctx context.Context, old *item.Instance, toID string, res *ExpireResult) error {
	target, err := e.lookup(toID)
	if err != nil {
		return err
	}
	now := e.now()

	var expireAt *time.Time
	if ex, ok := target.(item.Expirable); ok && ex.Duration() > 0 {
		t := now.Add(ex.Duration())
		expireAt = &t
	}
	bound := old.Bound
	if b, ok := target.(item.Bindable); ok {
		bound = b.BindOnGrant()
	}

	fresh := &item.Instance{
		ID:         e.newID(),
		ItemID:     toID,
		Owner:      old.Owner,
		Count:      old.Count,
		Bound:      bound,
		AcquiredAt: now,
		ExpireAt:   expireAt,
	}
	if err := e.store.Create(ctx, fresh); err != nil {
		return err
	}
	if err := e.store.Delete(ctx, old.ID, old.Version); err != nil {
		return err
	}

	res.Downgraded = append(res.Downgraded, Downgrade{
		InstanceID: old.ID, From: old.ItemID, To: toID, NewInstanceID: fresh.ID,
	})
	e.publish(ctx, event.KindDowngraded, old, old.Count, map[string]any{
		"to":              toID,
		"new_instance_id": fresh.ID,
	})
	return nil
}
