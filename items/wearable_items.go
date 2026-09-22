package items

import (
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/port"
)

// ============================================================
// 座驾
// 能力：可穿戴 + 被动属性 + 等级门槛 + 展示排序 (+ 可选时效)
// 没有 Use 方法，所以它在编译期就不是「可使用」的道具。
// ============================================================

// Mount 座驾：穿戴进 mount 槽，提供移速加成，可设等级门槛与时效。
type Mount struct {
	identity
	display
	wearable
	levelGate
	timed
	speed int64
}

// NewMount 构造一款座驾。
// duration 传 0 即永久款；传非 0 则为限时款，到期按 onExpire 处置。
func NewMount(id, name string, priority, rarity int, speed, minLevel int64,
	levels port.LevelSource, duration time.Duration, onExpire item.ExpirePolicy) *Mount {
	return &Mount{
		identity:  identity{id: id, name: name},
		display:   display{priority: priority, rarity: rarity},
		wearable:  wearable{slot: item.SlotMount, capacity: 1},
		levelGate: levelGate{minLevel: minLevel, levels: levels},
		timed:     timed{duration: duration, onExpire: onExpire},
		speed:     speed,
	}
}

// Modifiers 移速加成，穿上即生效。
func (m *Mount) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "move_speed", Value: m.speed}}
}

// ============================================================
// 头饰
// 能力：可穿戴 + 被动属性 + 展示排序 (+ 可选时效)
// ============================================================

// Headwear 头饰：穿戴进 headwear 槽，提供一个外观修饰符。
type Headwear struct {
	identity
	display
	wearable
	timed
	glow string
}

// NewHeadwear 构造一款头饰。限时款到期通常用 ExpireUnequip：卸下但留在背包。
func NewHeadwear(id, name string, priority, rarity int, glow string,
	duration time.Duration, onExpire item.ExpirePolicy) *Headwear {
	return &Headwear{
		identity: identity{id: id, name: name},
		display:  display{priority: priority, rarity: rarity},
		wearable: wearable{slot: item.SlotHeadwear, capacity: 1},
		timed:    timed{duration: duration, onExpire: onExpire},
		glow:     glow,
	}
}

// Modifiers 头部特效。
func (h *Headwear) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "head_glow", Value: h.glow}}
}

// ============================================================
// 勋章
// 能力：可穿戴（容量 3，可同时戴多枚）+ 展示排序
// 没有 Passive —— 勋章只是展示，不给属性。
// ============================================================

// Badge 勋章：badge 槽容量为 3，同一玩家可同时佩戴多枚。
type Badge struct {
	identity
	display
	wearable
}

// NewBadge 构造一枚勋章。
func NewBadge(id, name string, priority, rarity int) *Badge {
	return &Badge{
		identity: identity{id: id, name: name},
		display:  display{priority: priority, rarity: rarity},
		wearable: wearable{slot: item.SlotBadge, capacity: 3},
	}
}

// ============================================================
// 铭牌
// 能力：可穿戴（独占）+ 展示排序 (+ 可选时效)
// ============================================================

// NamePlate 铭牌：nameplate 槽独占，展示在昵称旁。
type NamePlate struct {
	identity
	display
	wearable
	timed
}

// NewNamePlate 构造一款铭牌。
func NewNamePlate(id, name string, priority, rarity int,
	duration time.Duration, onExpire item.ExpirePolicy) *NamePlate {
	return &NamePlate{
		identity: identity{id: id, name: name},
		display:  display{priority: priority, rarity: rarity},
		wearable: wearable{slot: item.SlotNamePlate, capacity: 1},
		timed:    timed{duration: duration, onExpire: onExpire},
	}
}

// ============================================================
// 聊天气泡
// 能力：可穿戴（独占）+ 被动属性 + 展示排序
// ============================================================

// ChatBubble 聊天气泡：改变玩家在聊天场景中的气泡样式。
type ChatBubble struct {
	identity
	display
	wearable
	style string
}

// NewChatBubble 构造一款聊天气泡。
func NewChatBubble(id, name string, priority, rarity int, style string) *ChatBubble {
	return &ChatBubble{
		identity: identity{id: id, name: name},
		display:  display{priority: priority, rarity: rarity},
		wearable: wearable{slot: item.SlotChatBubble, capacity: 1},
		style:    style,
	}
}

// Modifiers 气泡样式。
func (c *ChatBubble) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "bubble_style", Value: c.style}}
}

// ============================================================
// 头像框
// 能力：可穿戴（独占）+ 展示排序
// ============================================================

// Avatar 头像框。
type Avatar struct {
	identity
	display
	wearable
	timed
}

// NewAvatar 构造一款头像框。
func NewAvatar(id, name string, priority, rarity int,
	duration time.Duration, onExpire item.ExpirePolicy) *Avatar {
	return &Avatar{
		identity: identity{id: id, name: name},
		display:  display{priority: priority, rarity: rarity},
		wearable: wearable{slot: item.SlotAvatar, capacity: 1},
		timed:    timed{duration: duration, onExpire: onExpire},
	}
}
