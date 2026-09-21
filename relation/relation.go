// Package relation 提供双主体关系（CP/师徒/闺蜜等）的模型、仓储接口与事件。
// 关系是"两个玩家之间的契约"，独立于道具存在；道具类型通过 item.RelationGated
// 声明对关系的依赖，关系的建立/解除/到期由通用库存层驱动并联动卸下相关道具。
package relation

import (
	"context"
	"errors"
	"time"

	"github.com/Linuxea/item-system/event"
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

// 关系相关错误。
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

// Repo 关系仓储接口。FindActive/ListExpired 内部按时间过滤过期关系，
// 因此实现方需要自己的时钟接缝（见 memory.RelationRepo.UseClock）。
type Repo interface {
	// Save 新建关系；ID 已存在视为冲突。
	Save(ctx context.Context, r *Relation) error
	// Get 按 ID 取关系。
	Get(ctx context.Context, id string) (*Relation, error)
	// FindActive 查玩家在某类型下的生效关系；不存在或已过期返回 ErrNotFound。
	FindActive(ctx context.Context, owner string, t Type) (*Relation, error)
	// ListExpired 取 before 时刻前到期的生效关系。
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*Relation, error)
	// Update 以乐观锁条件更新关系（CAS）。
	Update(ctx context.Context, r *Relation, expectVersion int64) error
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
	// RelationID 关系标识。
	RelationID string
	// Type 关系类型。
	Type Type
	// PartyA 主体 A。
	PartyA string
	// PartyB 主体 B。
	PartyB string
}

// Name 返回事件名。
func (e Bound) Name() string { return EventBound }

// Dissolved 关系解除事件，ByExpiry 区分主动解除与到期自动解除。
type Dissolved struct {
	event.Base
	// RelationID 关系标识。
	RelationID string
	// Type 关系类型。
	Type Type
	// PartyA 主体 A。
	PartyA string
	// PartyB 主体 B。
	PartyB string
	// ByExpiry 是否因到期自动解除。
	ByExpiry bool
}

// Name 返回事件名。
func (e Dissolved) Name() string { return EventDissolved }
