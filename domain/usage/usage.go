// Package usage 提供道具主动使用的领域服务：校验 -> 物化效果 -> 执行 -> 扣减。
package usage

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

var (
	// ErrInstanceNotFound 实例不存在。
	ErrInstanceNotFound = errors.New("instance not found")
	// ErrNotOwner 实例不属于该玩家。
	ErrNotOwner = errors.New("instance does not belong to owner")
	// ErrNotUsable 道具不具备可使用行为。
	ErrNotUsable = errors.New("item is not usable")
	// ErrNotAvailable 实例当前不可用（已穿戴或已过期）。
	ErrNotAvailable = errors.New("item is not available")
	// ErrInsufficient 实例数量不足。
	ErrInsufficient = errors.New("insufficient item count")
	// ErrIdempotency 幂等键重复（疑似重复请求）。
	ErrIdempotency = errors.New("duplicate idempotency key")
)

// Clock 时钟端口，测试可注入固定时钟。
type Clock func() time.Time

// Request 使用请求。
type Request struct {
	// Owner 使用者，须与实例归属一致。
	Owner string
	// InstanceID 目标实例。
	InstanceID string
	// Count 使用数量，0 视为 1。
	Count int64
	// IdempotencyKey 幂等键，非空时防重复使用；业务失败会自动释放。
	IdempotencyKey string
	// Params 用户动态输入（如飘屏文案），止步于物化缝，不进入执行缝。
	Params map[string]any
}

// instanceStore 消费侧窄接口：本服务只使用实例仓储的三个方法。
type instanceStore interface {
	Get(ctx context.Context, id string) (*model.ItemInstance, error)
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
	Delete(ctx context.Context, id string) error
}

// Service 使用领域服务。
type Service struct {
	templates   repository.TemplateSource
	instances   instanceStore
	idempotency repository.IdempotencyStore
	publisher   event.Publisher
	compiler    behavior.Compiler
	now         Clock
}

// NewService 构造使用服务，依赖全部经由参数注入。
func NewService(
	templates repository.TemplateSource,
	instances instanceStore,
	idempotency repository.IdempotencyStore,
	publisher event.Publisher,
	compiler behavior.Compiler,
	now Clock,
) *Service {
	return &Service{
		templates:   templates,
		instances:   instances,
		idempotency: idempotency,
		publisher:   publisher,
		compiler:    compiler,
		now:         now,
	}
}

// Use 执行一次使用：幂等抢占 -> 归属/状态/数量校验 -> 编译取 Usable 组件
// -> 物化（缺参数零副作用失败）-> 逐次执行效果 -> 版本 CAS 扣减 -> 发布事件。
// 效果全部成功后才扣减，保证不出现"扣了道具没给效果"。
func (s *Service) Use(ctx context.Context, req Request) (*model.UseResult, error) {
	if req.Count <= 0 {
		req.Count = 1
	}
	if req.IdempotencyKey != "" {
		ok, err := s.idempotency.Claim(ctx, req.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrIdempotency
		}
	}

	inst, err := s.instances.Get(ctx, req.InstanceID)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	if inst.Owner != req.Owner {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrNotOwner
	}
	if !inst.Available(s.now()) {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrNotAvailable
	}

	tpl, err := s.templates.Get(ctx, inst.TemplateID)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	compiled, err := s.compiler.Compile(tpl)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	usable, ok := compiled.Usable()
	if !ok {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrNotUsable
	}
	if inst.Count < req.Count {
		s.release(ctx, req.IdempotencyKey)
		return nil, ErrInsufficient
	}

	// 物化缝：用户参数在这里绑定进命令；缺失在任何副作用前失败。
	cmds, err := effect.Materialize(usable.Effects, req.Params)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	// 每消耗一个单位执行一轮效果（宝箱开 3 次即抽 3 次）。
	for i := int64(0); i < req.Count; i++ {
		if err := effect.Execute(ctx, req.Owner, cmds); err != nil {
			s.release(ctx, req.IdempotencyKey)
			return nil, err
		}
	}

	// 扣减：乐观锁写入；扣到 0 直接删除实例。
	expect := inst.Version
	inst.Count -= req.Count
	inst.BumpVersion()
	if inst.Count == 0 {
		if err := s.instances.Delete(ctx, inst.ID); err != nil {
			return nil, err
		}
	} else if err := s.instances.Update(ctx, inst, expect); err != nil {
		return nil, err
	}

	s.publisher.Publish(event.Consumed{
		Base:       event.Base{At: s.now()},
		Owner:      req.Owner,
		TemplateID: inst.TemplateID,
		InstanceID: inst.ID,
		Count:      req.Count,
	})
	return &model.UseResult{InstanceID: inst.ID, Consumed: req.Count}, nil
}

// release 业务失败时释放幂等键，允许调用方重试；释放失败静默忽略（不掩盖原始错误）。
func (s *Service) release(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_ = s.idempotency.Release(ctx, key)
}
