// Package grant 提供道具发放领域服务：堆叠合并、时效设置、幂等防重。
package grant

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

var (
	// ErrInvalidCount 发放数量必须为正。
	ErrInvalidCount = errors.New("grant count must be positive")
	// ErrTemplateNotFound 模板不存在。
	ErrTemplateNotFound = errors.New("template not found")
	// ErrIdempotency 幂等键重复（疑似重复请求）。
	ErrIdempotency = errors.New("duplicate idempotency key")
)

// Clock 时钟端口，测试可注入固定时钟。
type Clock func() time.Time

// Request 发放请求。
type Request struct {
	// Owner 目标玩家。
	Owner string
	// TemplateID 道具模板。
	TemplateID string
	// Count 发放数量。
	Count int64
	// Source 发放来路（gm/shop/effect 等），进事件留痕。
	Source string
	// Reason 发放原因，进事件留痕。
	Reason string
	// IdempotencyKey 幂等键，非空时防重复发放；业务失败会自动释放。
	IdempotencyKey string
	// ExpireAt 显式过期时间；非空时优先于模板自身的时效配置。
	ExpireAt *time.Time
}

// instanceStore 消费侧窄接口：本服务只使用实例仓储的三个方法，
// 提供方（如 memory.InstanceRepo）以结构化类型自动满足。
// 想调用切片外的方法在这里编译不过——解耦由类型系统强制。
type instanceStore interface {
	ListByOwner(ctx context.Context, owner string) ([]*model.ItemInstance, error)
	Save(ctx context.Context, inst *model.ItemInstance) error
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
}

// Service 发放领域服务。
type Service struct {
	templates   repository.TemplateSource
	instances   instanceStore
	idempotency repository.IdempotencyStore
	publisher   event.Publisher
	compiler    behavior.Compiler
	newID       func() string
	now         Clock
}

// NewService 构造发放服务，依赖全部经由参数注入。
func NewService(
	templates repository.TemplateSource,
	instances instanceStore,
	idempotency repository.IdempotencyStore,
	publisher event.Publisher,
	compiler behavior.Compiler,
	newID func() string,
	now Clock,
) *Service {
	return &Service{
		templates:   templates,
		instances:   instances,
		idempotency: idempotency,
		publisher:   publisher,
		compiler:    compiler,
		newID:       newID,
		now:         now,
	}
}

// Grant 执行一次发放：幂等抢占 -> 取模板并编译 -> 应用（堆叠/新建）-> 发布事件。
// 任何一步失败都释放幂等键，保证失败请求可安全重试。
func (s *Service) Grant(ctx context.Context, req Request) (*model.GrantResult, error) {
	if req.Count <= 0 {
		return nil, ErrInvalidCount
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

	tpl, err := s.templates.Get(ctx, req.TemplateID)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}
	compiled, err := s.compiler.Compile(tpl)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	primaryID, err := s.apply(ctx, req, compiled)
	if err != nil {
		s.release(ctx, req.IdempotencyKey)
		return nil, err
	}

	s.publisher.Publish(event.Granted{
		Base:       event.Base{At: s.now()},
		Owner:      req.Owner,
		TemplateID: req.TemplateID,
		InstanceID: primaryID,
		Count:      req.Count,
		Source:     req.Source,
		Reason:     req.Reason,
	})
	return &model.GrantResult{InstanceID: primaryID, Count: req.Count}, nil
}

// apply 落库发放数量：先尝试并入既有堆叠（逐个 CAS 更新），余量再新建实例。
// 返回主实例 ID：优先取被合并的第一个实例，否则取新实例。
func (s *Service) apply(ctx context.Context, req Request, compiled *behavior.Compiled) (string, error) {
	now := s.now()

	stack, _ := compiled.Stackable()
	expirable, hasExpiry := compiled.Expirable()
	bindable, _ := compiled.Bindable()

	remain := req.Count
	var primaryID string

	// 仅当可堆叠（MaxStack > 1）时尝试合并，减少实例数量。
	if stack.MaxStack > 1 {
		existing, err := s.instances.ListByOwner(ctx, req.Owner)
		if err != nil {
			return "", err
		}
		for _, inst := range existing {
			if remain == 0 {
				break
			}
			if !s.canMergeInto(inst, req, stack, now) {
				continue
			}
			room := stack.MaxStack - inst.Count
			if room <= 0 {
				continue
			}
			take := min(remain, room)
			// 乐观锁写入：以读取到的版本为期望值，并发冲突会失败。
			expect := inst.Version
			inst.Count += take
			inst.BumpVersion()
			if err := s.instances.Update(ctx, inst, expect); err != nil {
				return "", err
			}
			remain -= take
			if primaryID == "" {
				primaryID = inst.ID
			}
		}
	}

	// 余量新建实例；过期时间取请求显式值，否则用模板时效配置。
	if remain > 0 {
		inst := &model.ItemInstance{
			ID:         s.newID(),
			TemplateID: req.TemplateID,
			Owner:      req.Owner,
			Count:      remain,
			Bound:      bindable.BindOnPickup,
			Status:     model.StatusNormal,
			AcquiredAt: now,
			Version:    0,
		}
		switch {
		case req.ExpireAt != nil:
			t := *req.ExpireAt
			inst.ExpireAt = &t
		case hasExpiry && expirable.Duration > 0:
			t := now.Add(expirable.Duration)
			inst.ExpireAt = &t
		}
		if err := s.instances.Save(ctx, inst); err != nil {
			return "", err
		}
		if primaryID == "" {
			primaryID = inst.ID
		}
	}
	return primaryID, nil
}

// canMergeInto 判断发放量能否并入既有实例：同模板、当前可用、且时效窗口一致
// （时效不同的堆叠道具分开存放，避免"永久+限时"混在一个实例里）。
func (s *Service) canMergeInto(inst *model.ItemInstance, req Request, stack behavior.Stackable, now time.Time) bool {
	if inst.TemplateID != req.TemplateID || !inst.Available(now) {
		return false
	}
	if !sameExpiry(inst.ExpireAt, req.ExpireAt) {
		return false
	}
	return true
}

// sameExpiry 比较两个可空过期时间是否指向同一时刻。
func sameExpiry(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

// release 业务失败时释放幂等键，允许调用方重试；释放失败静默忽略（不掩盖原始错误）。
func (s *Service) release(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_ = s.idempotency.Release(ctx, key)
}
