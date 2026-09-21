// events.go 道具领域的领域事件：由库存层在状态变更成功后发布，
// 用于解耦后续通知、统计等消费方。
package item

import (
	"github.com/Linuxea/item-system/event"
)

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

// Granted 道具发放事件。Source/Reason 记录发放来路（gm/shop/effect 等），
// 便于审计与追溯。
type Granted struct {
	event.Base
	// Owner 玩家。
	Owner string
	// DefID 道具定义标识。
	DefID string
	// InstanceID 主实例标识。
	InstanceID string
	// Count 发放数量。
	Count int64
	// Source 发放来路。
	Source string
	// Reason 发放原因。
	Reason string
}

// Name 返回事件名。
func (e Granted) Name() string { return ItemGranted }

// Consumed 道具使用消耗事件。
type Consumed struct {
	event.Base
	// Owner 玩家。
	Owner string
	// DefID 道具定义标识。
	DefID string
	// InstanceID 被消费的实例。
	InstanceID string
	// Count 消费数量。
	Count int64
}

// Name 返回事件名。
func (e Consumed) Name() string { return ItemConsumed }

// Equipped 道具穿戴事件。
type Equipped struct {
	event.Base
	// Owner 玩家。
	Owner string
	// DefID 道具定义标识。
	DefID string
	// InstanceID 实例标识。
	InstanceID string
	// Slot 穿戴槽位。
	Slot string
}

// Name 返回事件名。
func (e Equipped) Name() string { return ItemEquipped }

// Unequipped 道具卸下事件。
type Unequipped struct {
	event.Base
	// Owner 玩家。
	Owner string
	// DefID 道具定义标识。
	DefID string
	// InstanceID 实例标识。
	InstanceID string
	// Slot 穿戴槽位。
	Slot string
}

// Name 返回事件名。
func (e Unequipped) Name() string { return ItemUnequipped }

// Expired 道具过期事件，Policy 为实际执行的过期策略（remove/unequip/downgrade）。
type Expired struct {
	event.Base
	// Owner 玩家。
	Owner string
	// DefID 道具定义标识（降级后为新定义标识）。
	DefID string
	// InstanceID 实例标识。
	InstanceID string
	// Policy 过期策略。
	Policy string
}

// Name 返回事件名。
func (e Expired) Name() string { return ItemExpired }
