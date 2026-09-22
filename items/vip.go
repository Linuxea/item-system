package items

import (
	"time"

	"github.com/Linuxea/item-system/item"
)

// ============================================================
// VIP 会员
// 能力：可穿戴（vip 槽独占）+ 被动属性 + 时效 + 降级 + 展示排序
//
// 降级链的关键：每一款 VIP 只知道自己的下一环是谁。
// vip_month -> vip_week -> vip_trial -> (删除)
// 引擎在处置到期实例时读 DowngradeTo()，按目标自己的时效重新计时，
// 链条自然延续，没有任何一处需要知道整条链长什么样。
// ============================================================

// VIP 会员身份道具。
type VIP struct {
	identity
	display
	wearable
	timed
	level       int64
	downgradeTo string
}

// NewVIP 构造一档会员。
// downgradeTo 为空表示到期直接删除；非空则到期换成该款道具。
func NewVIP(id, name string, priority, rarity int, level int64,
	duration time.Duration, downgradeTo string) *VIP {
	policy := item.ExpireRemove
	if downgradeTo != "" {
		policy = item.ExpireDowngrade
	}
	return &VIP{
		identity:    identity{id: id, name: name},
		display:     display{priority: priority, rarity: rarity},
		wearable:    wearable{slot: item.SlotVIP, capacity: 1},
		timed:       timed{duration: duration, onExpire: policy},
		level:       level,
		downgradeTo: downgradeTo,
	}
}

// Modifiers 会员等级，展示与权益判定都读它。
func (v *VIP) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "vip_level", Value: v.level}}
}

// DowngradeTo 到期后降级成哪一款。
// 仅在 OnExpire 返回 ExpireDowngrade 时被引擎读取。
func (v *VIP) DowngradeTo() string { return v.downgradeTo }
