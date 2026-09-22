// Package event 定义道具体系对外发出的领域事件。
//
// 引擎在状态变更成功后发布事件，订阅方（任务系统、数据上报、风控）自行消费。
// 发布失败不回滚业务 —— 事件是通知，不是事务的一部分。
package event

import (
	"context"
	"time"
)

// Kind 事件类型。
type Kind string

const (
	// KindGranted 道具已发放。
	KindGranted Kind = "item.granted"
	// KindConsumed 道具已被使用消耗。
	KindConsumed Kind = "item.consumed"
	// KindEquipped 道具已穿戴。
	KindEquipped Kind = "item.equipped"
	// KindUnequipped 道具已卸下。
	KindUnequipped Kind = "item.unequipped"
	// KindExpired 道具已过期（并按策略处置完毕）。
	KindExpired Kind = "item.expired"
	// KindDowngraded 道具已降级为另一款道具。
	KindDowngraded Kind = "item.downgraded"
	// KindRelationBound 关系已建立。
	KindRelationBound Kind = "relation.bound"
	// KindRelationDissolved 关系已解除。
	KindRelationDissolved Kind = "relation.dissolved"
)

// Event 领域事件。Extra 承载各事件特有的少量上下文（如降级目标、解除原因）。
type Event struct {
	// Kind 事件类型。
	Kind Kind
	// Owner 相关玩家。
	Owner string
	// ItemID 相关道具 ID，关系类事件为空。
	ItemID string
	// InstanceID 相关实例 ID，关系类事件为空。
	InstanceID string
	// Count 涉及数量。
	Count int64
	// At 事件发生时间。
	At time.Time
	// Extra 事件特有的补充字段。
	Extra map[string]any
}

// Publisher 事件发布端口，由基础设施实现。
type Publisher interface {
	// Publish 发布一条领域事件。
	Publish(ctx context.Context, e Event)
}

// NopPublisher 空发布器：不需要事件时使用，避免调用方到处判空。
type NopPublisher struct{}

// Publish 丢弃事件。
func (NopPublisher) Publish(context.Context, Event) {}
