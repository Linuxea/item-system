// Package relation 提供双主体关系领域服务（CP/师徒/闺蜜等）。
// 关系是"两个玩家之间的契约"，独立于道具聚合根存在；
// 道具侧通过 Condition.RequiresRelation 引用关系作为穿戴前置，
// 关系解除时由应用层联动卸下相关道具。
package relation

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/event"
)

// Type 关系类型。
type Type string

const (
	// TypeCP 情侣关系。
	TypeCP Type = "cp"
	// TypeMaster 师徒关系。
	TypeMaster Type = "master"
	// TypeBestie 闺蜜关系。
	TypeBestie Type = "bestie"
)

// Status 关系状态。
type Status string

const (
	// StatusActive 生效中。
	StatusActive Status = "active"
	// StatusDissolved 已解除（软删除，保留追溯）。
	StatusDissolved Status = "dissolved"
)

var (
	// ErrNotFound 关系不存在或无生效关系。
	ErrNotFound = errors.New("relation not found")
	// ErrSelfBinding 不允许与自己建立关系。
	ErrSelfBinding = errors.New("cannot bind relation to self")
	// ErrAlreadyBound 任一方在此类型下已有生效关系。
	ErrAlreadyBound = errors.New("relation already bound")
	// ErrDissolved 关系已解除，不可重复解除。
	ErrDissolved = errors.New("relation dissolved")
)

// Relation 关系聚合根：双主体（PartyA/PartyB）、可选时效、乐观锁版本号。
type Relation struct {
	// ID 关系唯一标识。
	ID string
	// Type 关系类型。
	Type Type
	// PartyA 主体 A。
	PartyA string
	// PartyB 主体 B。
	PartyB string
	// Status 关系状态。
	Status Status
	// CreatedAt 建立时间。
	CreatedAt time.Time
	// ExpireAt 过期时间，nil 表示永久。
	ExpireAt *time.Time
	// Version 乐观锁版本号。
	Version int64
}

// Expired 报告关系在给定时刻是否已过期；永久关系不过期。
func (r *Relation) Expired(now time.Time) bool {
	return r.ExpireAt != nil && !now.Before(*r.ExpireAt)
}

// Involves 报告玩家是否为关系主体之一。
func (r *Relation) Involves(owner string) bool {
	return r.PartyA == owner || r.PartyB == owner
}

// BumpVersion 将乐观锁版本号自增并返回新值；写入仓储前调用。
func (r *Relation) BumpVersion() int64 {
	r.Version++
	return r.Version
}

// 关系事件名称常量。
const (
	// EventBound 关系建立。
	EventBound = "relation.bound"
	// EventDissolved 关系解除（主动或过期）。
	EventDissolved = "relation.dissolved"
)

// Bound 关系建立事件。
type Bound struct {
	event.Base
	RelationID string
	Type       Type
	PartyA     string
	PartyB     string
}

func (e Bound) Name() string { return EventBound }

// Dissolved 关系解除事件，ByExpiry 区分主动解除与到期自动解除。
type Dissolved struct {
	event.Base
	RelationID string
	Type       Type
	PartyA     string
	PartyB     string
	ByExpiry   bool
}

func (e Dissolved) Name() string { return EventDissolved }

// Repo 关系仓储的提供方接口。
// 注：此接口相对宽（含 Save/Update 写方法），接入真实存储时可按消费侧继续收窄。
type Repo interface {
	Save(ctx context.Context, r *Relation) error
	Get(ctx context.Context, id string) (*Relation, error)
	FindActive(ctx context.Context, owner string, t Type) (*Relation, error)
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*Relation, error)
	Update(ctx context.Context, r *Relation, expectVersion int64) error
}

// Clock 时钟端口，测试可注入固定时钟。
type Clock func() time.Time

// Service 关系领域服务。
type Service struct {
	repo      Repo
	publisher event.Publisher
	newID     func() string
	now       Clock
}

// NewService 构造关系服务，依赖全部经由参数注入。
func NewService(repo Repo, publisher event.Publisher, newID func() string, now Clock) *Service {
	return &Service{repo: repo, publisher: publisher, newID: newID, now: now}
}

// Bind 建立关系：禁止自绑，且任一方在此类型下已有生效关系即拒绝（独占语义）。
func (s *Service) Bind(ctx context.Context, t Type, partyA, partyB string, expireAt *time.Time) (*Relation, error) {
	if partyA == partyB {
		return nil, ErrSelfBinding
	}
	for _, party := range []string{partyA, partyB} {
		if _, err := s.repo.FindActive(ctx, party, t); !errors.Is(err, ErrNotFound) {
			return nil, ErrAlreadyBound
		}
	}

	r := &Relation{
		ID:        s.newID(),
		Type:      t,
		PartyA:    partyA,
		PartyB:    partyB,
		Status:    StatusActive,
		CreatedAt: s.now(),
		ExpireAt:  expireAt,
	}
	if err := s.repo.Save(ctx, r); err != nil {
		return nil, err
	}

	s.publishBound(r)
	return r, nil
}

// Dissolve 主动解除关系：状态置为已解除并清空时效（版本 CAS），幂等拒绝重复解除。
// 道具联动（卸下关系饰品）由应用层订阅完成，关系服务不感知道具。
func (s *Service) Dissolve(ctx context.Context, id string) error {
	r, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if r.Status == StatusDissolved {
		return ErrDissolved
	}
	expect := r.Version
	r.Status = StatusDissolved
	r.ExpireAt = nil
	r.BumpVersion()
	if err := s.repo.Update(ctx, r, expect); err != nil {
		return err
	}
	s.publishDissolved(r, false)
	return nil
}

// RunExpiry 批量解除已到期关系，limit 控制单批规模；返回实际处理数。
func (s *Service) RunExpiry(ctx context.Context, limit int) (int, error) {
	expired, err := s.repo.ListExpired(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, r := range expired {
		expect := r.Version
		r.Status = StatusDissolved
		r.ExpireAt = nil
		r.BumpVersion()
		if err := s.repo.Update(ctx, r, expect); err != nil {
			return processed, err
		}
		s.publishDissolved(r, true)
		processed++
	}
	return processed, nil
}

// publishBound 发布关系建立事件。
func (s *Service) publishBound(r *Relation) {
	s.publisher.Publish(Bound{
		Base:       event.Base{At: s.now()},
		RelationID: r.ID,
		Type:       r.Type,
		PartyA:     r.PartyA,
		PartyB:     r.PartyB,
	})
}

// publishDissolved 发布关系解除事件。
func (s *Service) publishDissolved(r *Relation, byExpiry bool) {
	s.publisher.Publish(Dissolved{
		Base:       event.Base{At: s.now()},
		RelationID: r.ID,
		Type:       r.Type,
		PartyA:     r.PartyA,
		PartyB:     r.PartyB,
		ByExpiry:   byExpiry,
	})
}
