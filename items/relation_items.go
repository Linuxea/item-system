package items

import (
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/port"
)

// ============================================================
// 关系卡 / CP 戒指
//
// 这两款道具的特别之处：它们要求玩家先与他人建立关系才能穿戴。
// 这不需要引擎懂「关系」是什么 —— 道具嵌入 relationGate 片段，
// 自己去问 RelationChecker 端口，引擎照旧只调用 CanEquip。
//
// 关系解除时的联动卸下也不在道具里：那是 relation 包在解除关系后
// 调用 engine.UnequipSlot 完成的。
// ============================================================

// RelationCard 关系卡：展示双方的关系（好友、师徒、结拜等）。
type RelationCard struct {
	identity
	display
	wearable
	relationGate
	timed
	label string
}

// NewRelationCard 构造一款关系卡。requires 为所需的关系类型。
func NewRelationCard(id, name string, priority, rarity int, requires, label string,
	checker port.RelationChecker, duration time.Duration, onExpire item.ExpirePolicy) *RelationCard {
	return &RelationCard{
		identity:     identity{id: id, name: name},
		display:      display{priority: priority, rarity: rarity},
		wearable:     wearable{slot: item.SlotRelation, capacity: 1},
		relationGate: relationGate{requires: requires, checker: checker},
		timed:        timed{duration: duration, onExpire: onExpire},
		label:        label,
	}
}

// Modifiers 关系标签。
func (r *RelationCard) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "relation_label", Value: r.label}}
}

// CPRing CP 戒指：需要 cp 关系才能戴，且发放即绑定（不可交易）。
type CPRing struct {
	identity
	display
	wearable
	relationGate
	shine int64
}

// NewCPRing 构造一款 CP 戒指。
func NewCPRing(id, name string, priority, rarity int, shine int64, checker port.RelationChecker) *CPRing {
	return &CPRing{
		identity:     identity{id: id, name: name},
		display:      display{priority: priority, rarity: rarity},
		wearable:     wearable{slot: item.SlotCPRing, capacity: 1},
		relationGate: relationGate{requires: "cp", checker: checker},
		shine:        shine,
	}
}

// Modifiers 戒指光效强度。
func (c *CPRing) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "ring_shine", Value: c.shine}}
}

// BindOnGrant CP 戒指发放即绑定，不可交易。
func (c *CPRing) BindOnGrant() bool { return true }
