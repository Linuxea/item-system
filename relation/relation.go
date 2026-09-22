// Package relation 管理玩家之间的双主体关系（CP、挚友、师徒）。
//
// 关系是独立的聚合根，不是道具的一个字段：一段关系属于两个玩家，
// 而一件道具只属于一个玩家，两者的生命周期本就不同。
// 道具通过 port.RelationChecker 查询关系，关系解除时反过来联动卸下相关道具。
package relation

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
)

// Relation 一段双主体关系。
type Relation struct {
	// ID 关系唯一标识。
	ID string
	// Type 关系类型：cp / bestie / master。
	Type string
	// A、B 关系双方，无先后之分。
	A string
	B string
	// CreatedAt 建立时间。
	CreatedAt time.Time
	// ExpireAt 到期时间，nil 表示长期有效。
	ExpireAt *time.Time
	// Dissolved 是否已解除。
	Dissolved bool
}

// Involves 报告某玩家是否是这段关系的一方。
func (r *Relation) Involves(owner string) bool { return r.A == owner || r.B == owner }

// Other 返回关系中的另一方；owner 不在这段关系里时返回空串。
func (r *Relation) Other(owner string) string {
	switch owner {
	case r.A:
		return r.B
	case r.B:
		return r.A
	default:
		return ""
	}
}

// Active 报告关系在给定时刻是否有效。
func (r *Relation) Active(now time.Time) bool {
	if r.Dissolved {
		return false
	}
	return r.ExpireAt == nil || now.Before(*r.ExpireAt)
}

// Repo 关系仓储端口。
type Repo interface {
	// Save 新建关系。
	Save(ctx context.Context, r *Relation) error
	// Get 按 ID 取关系。
	Get(ctx context.Context, id string) (*Relation, error)
	// Update 更新关系。
	Update(ctx context.Context, r *Relation) error
	// ListByOwner 列出与某玩家相关的全部关系（含已解除与已过期）。
	ListByOwner(ctx context.Context, owner string) ([]*Relation, error)
	// ListAll 列出全部关系，供过期扫描使用。
	ListAll(ctx context.Context) ([]*Relation, error)
}

// Unequipper 解除关系时联动卸下道具的端口。
// engine.Engine 天然满足它（有同名同签名的方法），无需任何适配代码。
type Unequipper interface {
	UnequipSlot(ctx context.Context, owner string, slot item.Slot) ([]string, error)
}

// Options 关系服务构造参数。
type Options struct {
	// Repo 关系仓储，必填。
	Repo Repo
	// Unequipper 联动卸下器；为 nil 时解除关系不卸道具。
	Unequipper Unequipper
	// SlotsOnDissolve 各关系类型解除时需要卸下的槽位；未配置的类型用 DefaultSlots。
	SlotsOnDissolve map[string][]item.Slot
	// Publisher 事件发布器。
	Publisher event.Publisher
	// Now 时钟。
	Now func() time.Time
}

// DefaultSlots 关系解除时默认需要卸下的槽位。
var DefaultSlots = []item.Slot{item.SlotRelation, item.SlotCPRing}

// Service 关系服务。
type Service struct {
	mu      sync.RWMutex
	repo    Repo
	unequip Unequipper
	slots   map[string][]item.Slot
	bus     event.Publisher
	now     func() time.Time
	seq     int64
}

// New 构造关系服务。
func New(opts Options) *Service {
	s := &Service{
		repo:    opts.Repo,
		unequip: opts.Unequipper,
		slots:   opts.SlotsOnDissolve,
		bus:     opts.Publisher,
		now:     opts.Now,
	}
	if s.bus == nil {
		s.bus = event.NopPublisher{}
	}
	if s.now == nil {
		s.now = time.Now
	}
	return s
}

// BindUnequipper 注入联动卸下器。
//
// 与 engine.LateGranter 同理：道具需要关系服务、关系服务需要引擎、引擎需要道具，
// 三者成环。做法是关系服务先造出来交给道具，引擎构造完成后再回注。
func (s *Service) BindUnequipper(u Unequipper) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unequip = u
}

// unequipper 取当前的联动卸下器。
func (s *Service) unequipper() Unequipper {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.unequip
}

// Bind 建立一段关系。duration 为 0 表示长期有效。
//
// 注意：真实业务里建立关系通常需要对方同意，那属于上层的邀请/确认流程，
// 本方法只负责「双方已经谈妥」之后的落地。
func (s *Service) Bind(ctx context.Context, relType, a, b string, duration time.Duration) (*Relation, error) {
	if a == "" || b == "" || a == b {
		return nil, fmt.Errorf("%w: 关系双方不合法", item.ErrInvalidParam)
	}
	now := s.now()
	r := &Relation{
		ID:        fmt.Sprintf("rel_%d", atomic.AddInt64(&s.seq, 1)),
		Type:      relType,
		A:         a,
		B:         b,
		CreatedAt: now,
	}
	if duration > 0 {
		t := now.Add(duration)
		r.ExpireAt = &t
	}
	if err := s.repo.Save(ctx, r); err != nil {
		return nil, err
	}
	s.bus.Publish(ctx, event.Event{
		Kind: event.KindRelationBound, Owner: a, At: now,
		Extra: map[string]any{"relation_id": r.ID, "type": relType, "other": b},
	})
	return r, nil
}

// Dissolve 解除关系，并联动卸下双方与该关系绑定的道具。
func (s *Service) Dissolve(ctx context.Context, id, reason string) error {
	r, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if r.Dissolved {
		return nil
	}
	r.Dissolved = true
	if err := s.repo.Update(ctx, r); err != nil {
		return err
	}
	return s.afterEnd(ctx, r, reason)
}

// RunExpiry 扫描并处置已到期的关系，处置方式与主动解除一致。
func (s *Service) RunExpiry(ctx context.Context) error {
	all, err := s.repo.ListAll(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	for _, r := range all {
		if r.Dissolved || r.ExpireAt == nil || now.Before(*r.ExpireAt) {
			continue
		}
		r.Dissolved = true
		if err := s.repo.Update(ctx, r); err != nil {
			return err
		}
		if err := s.afterEnd(ctx, r, "expired"); err != nil {
			return err
		}
	}
	return nil
}

// afterEnd 关系终止后的联动：卸下双方相关槽位的道具，发事件。
func (s *Service) afterEnd(ctx context.Context, r *Relation, reason string) error {
	if u := s.unequipper(); u != nil {
		for _, owner := range []string{r.A, r.B} {
			for _, slot := range s.slotsFor(r.Type) {
				if _, err := u.UnequipSlot(ctx, owner, slot); err != nil {
					return err
				}
			}
		}
	}
	s.bus.Publish(ctx, event.Event{
		Kind: event.KindRelationDissolved, Owner: r.A, At: s.now(),
		Extra: map[string]any{"relation_id": r.ID, "type": r.Type, "other": r.B, "reason": reason},
	})
	return nil
}

// slotsFor 取某关系类型解除时要卸下的槽位。
func (s *Service) slotsFor(relType string) []item.Slot {
	if v, ok := s.slots[relType]; ok {
		return v
	}
	return DefaultSlots
}

// HasActive 报告玩家当前是否存在指定类型的有效关系。
// 本方法让 Service 结构化满足 port.RelationChecker，可直接注入给关系卡与 CP 戒指。
func (s *Service) HasActive(ctx context.Context, owner, relationType string) (bool, error) {
	list, err := s.repo.ListByOwner(ctx, owner)
	if err != nil {
		return false, err
	}
	now := s.now()
	for _, r := range list {
		if r.Type == relationType && r.Active(now) {
			return true, nil
		}
	}
	return false, nil
}
