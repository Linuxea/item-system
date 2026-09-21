// Package event 定义道具领域的领域事件与发布端口。
// 事件由领域服务在状态变更成功后发布，用于解耦后续通知、统计等消费方。
package event

import "time"

// 领域事件名称常量。
const (
	// ItemGranted 道具发放完成。
	ItemGranted = "item.granted"
	// ItemConsumed 道具使用消耗完成。
	ItemConsumed = "item.consumed"
	// ItemEquipped 道具穿戴完成。
	ItemEquipped = "item.equipped"
	// ItemUnequipped 道具卸下完成。
	ItemUnequipped = "item.unequipped"
	// ItemExpired 道具过期策略执行完成。
	ItemExpired = "item.expired"
)

// Event 领域事件接口：事件名 + 发生时间。
type Event interface {
	Name() string
	OccurredAt() time.Time
}

// Base 事件公共部分，内嵌到具体事件以获得 OccurredAt 实现。
type Base struct {
	At time.Time
}

func (b Base) OccurredAt() time.Time { return b.At }

// Granted 道具发放事件。Source/Reason 记录发放来路（gm/shop/effect 等），便于审计与追溯。
type Granted struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Count      int64
	Source     string
	Reason     string
}

func (e Granted) Name() string { return ItemGranted }

// Consumed 道具使用消耗事件。
type Consumed struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Count      int64
}

func (e Consumed) Name() string { return ItemConsumed }

// Equipped 道具穿戴事件。
type Equipped struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Slot       string
}

func (e Equipped) Name() string { return ItemEquipped }

// Unequipped 道具卸下事件。
type Unequipped struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Slot       string
}

func (e Unequipped) Name() string { return ItemUnequipped }

// Expired 道具过期事件，Policy 为实际执行的过期策略（remove/unequip/downgrade）。
type Expired struct {
	Base
	Owner      string
	TemplateID string
	InstanceID string
	Policy     string
}

func (e Expired) Name() string { return ItemExpired }

// Publisher 领域事件发布端口；实现方负责同步或异步分发。
type Publisher interface {
	Publish(events ...Event)
}
