// Package catalog 提供九类道具的模板示例集（纯配置数据）。
// 每类道具都是既有行为组件的组合，验证"新道具 = 数据 + 组合，不改核心"：
// 座驾（穿戴+被动+等级条件）、勋章（多容量槽）、头饰、VIP（降级链）、
// 关系卡（关系前置）、飘屏（动态参数）、聊天气泡（场景排序）、铭牌、CP 戒指（绑定+关系前置）。
package catalog

import (
	"github.com/Linuxea/item-system/domain/behavior"
	"github.com/Linuxea/item-system/domain/model"
	"github.com/Linuxea/item-system/domain/relation"
)

// Mounts 座驾模板：穿戴进 mount 槽，带移速被动与等级前置条件，7d 款到期即删。
func Mounts() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "mount_cloud", Category: model.CategoryMount, Name: "Cloud Steed", Priority: 65, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotMount), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "move_speed", "value": int64(80)}}},
				behavior.KeyCondition:  {"min_level": int64(1)},
			},
		},
		{
			ID: "mount_dragon", Category: model.CategoryMount, Name: "Dragon Chariot", Priority: 85, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotMount), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "move_speed", "value": int64(150)}}},
				behavior.KeyCondition:  {"min_level": int64(30)},
			},
		},
		{
			ID: "mount_ember_7d", Category: model.CategoryMount, Name: "Ember Racer (7d)", Priority: 70, Rarity: 4,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotMount), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "move_speed", "value": int64(120)}}},
				behavior.KeyCondition:  {"min_level": int64(10)},
				behavior.KeyExpirable:  {"duration": "168h", "on_expire": behavior.ExpirePolicyRemove},
			},
		},
	}
}

// Badges 勋章模板：badge 槽容量 3（多枚并穿），新手勋章限堆叠 1。
func Badges() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "badge_abyss", Category: model.CategoryBadge, Name: "Abyss Conqueror", Priority: 70, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotBadge), "capacity": 3},
			},
		},
		{
			ID: "badge_flame", Category: model.CategoryBadge, Name: "Flame Veteran", Priority: 70, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotBadge), "capacity": 3},
			},
		},
		{
			ID: "badge_rookie", Category: model.CategoryBadge, Name: "Rookie Star", Priority: 40, Rarity: 1,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotBadge), "capacity": 3},
				behavior.KeyStackable:  {"max_stack": int64(1)},
			},
		},
	}
}

// Headwear 头饰模板：穿戴进 headwear 槽，限时款到期仅卸下保留。
func Headwear() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "crown_aurora", Category: model.CategoryHeadwear, Name: "Aurora Crown", Priority: 88, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotHeadwear), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "head_glow", "value": "aurora"}}},
			},
		},
		{
			ID: "cap_sprint_3d", Category: model.CategoryHeadwear, Name: "Sprint Cap (3d)", Priority: 55, Rarity: 2,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotHeadwear), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "72h", "on_expire": behavior.ExpirePolicyUnequip},
			},
		},
	}
}

// VIPs VIP 模板：vip 槽容量 1，月卡到期降级为周卡、周卡降级为 vip0（链式降级），
// 试用卡到期直接删除。
func VIPs() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "vip3_month", Category: model.CategoryVIP, Name: "VIP3 Monthly", Priority: 60, Rarity: 4,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotVIP), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "720h", "on_expire": behavior.ExpirePolicyDowngrade, "downgrade_to": "vip1_week"},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "vip_level", "value": float64(3)}}},
			},
		},
		{
			ID: "vip1_week", Category: model.CategoryVIP, Name: "VIP1 Weekly", Priority: 30, Rarity: 2,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotVIP), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "168h", "on_expire": behavior.ExpirePolicyDowngrade, "downgrade_to": "vip0"},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "vip_level", "value": float64(1)}}},
			},
		},
		{
			ID: "vip0", Category: model.CategoryVIP, Name: "VIP0 Basic", Priority: 10, Rarity: 1,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotVIP), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "vip_level", "value": float64(0)}}},
			},
		},
		{
			ID: "vip_trial_1d", Category: model.CategoryVIP, Name: "VIP Trial (1d)", Priority: 30, Rarity: 1,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotVIP), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "24h", "on_expire": behavior.ExpirePolicyRemove},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "vip_level", "value": float64(1)}}},
			},
		},
	}
}

// RelationCards 关系卡模板：穿戴前置为对应类型的生效关系（师徒/闺蜜），
// 关系解除后由应用层联动卸下。
func RelationCards() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "card_master", Category: model.CategoryRelationCard, Name: "Master Card", Priority: 50, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotRelation), "capacity": 1},
				behavior.KeyCondition:  {"requires_relation": string(relation.TypeMaster)},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "mentor_tag", "value": true}}},
			},
		},
		{
			ID: "card_bestie", Category: model.CategoryRelationCard, Name: "Bestie Card", Priority: 50, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotRelation), "capacity": 2},
				behavior.KeyCondition:  {"requires_relation": string(relation.TypeBestie)},
			},
		},
	}
}

// Banners 飘屏模板：可堆叠消耗品，使用时需用户输入文案（param=text，
// 经物化缝绑定进 BroadcastBanner 命令）。
func Banners() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "banner_rose", Category: model.CategoryConsumable, Name: "Rose Banner", Priority: 0, Rarity: 2,
			Behaviors: map[string]map[string]any{
				behavior.KeyStackable: {"max_stack": int64(99)},
				behavior.KeyUsable: {"effects": []any{
					map[string]any{"kind": "broadcast_banner", "duration": "10s", "param": "text"},
				}},
			},
		},
		{
			ID: "banner_fire_30s", Category: model.CategoryConsumable, Name: "Fire Banner (30s)", Priority: 0, Rarity: 4,
			Behaviors: map[string]map[string]any{
				behavior.KeyStackable: {"max_stack": int64(99)},
				behavior.KeyUsable: {"effects": []any{
					map[string]any{"kind": "broadcast_banner", "duration": "30s", "param": "text"},
				}},
			},
		},
	}
}

// ChatBubbles 聊天气泡模板：穿戴进 chat_bubble 槽，限时款到期仅卸下保留。
func ChatBubbles() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "bubble_star", Category: model.CategoryChatBubble, Name: "Star Bubble", Priority: 45, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotChatBubble), "capacity": 1},
			},
		},
		{
			ID: "bubble_aurora", Category: model.CategoryChatBubble, Name: "Aurora Bubble", Priority: 75, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotChatBubble), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "168h", "on_expire": behavior.ExpirePolicyUnequip},
			},
		},
	}
}

// Nameplates 铭牌模板：穿戴进 nameplate 槽，节日款到期即删。
func Nameplates() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "np_ember", Category: model.CategoryNamePlate, Name: "Ember Plate", Priority: 80, Rarity: 4,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotNamePlate), "capacity": 1},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "plate_color", "value": "ember"}}},
			},
		},
		{
			ID: "np_festival_72h", Category: model.CategoryNamePlate, Name: "Festival Plate (72h)", Priority: 82, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotNamePlate), "capacity": 1},
				behavior.KeyExpirable:  {"duration": "72h", "on_expire": behavior.ExpirePolicyRemove},
			},
		},
	}
}

// CPRings CP 戒指模板：拾取即绑定，穿戴前置为生效 CP 关系；
// 由应用层 BindCP 流程成对发放并自动穿戴。
func CPRings() []*model.ItemTemplate {
	return []*model.ItemTemplate{
		{
			ID: "ring_cp_diamond", Category: model.CategoryCPRing, Name: "CP Diamond Ring", Priority: 95, Rarity: 5,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotCPRing), "capacity": 1},
				behavior.KeyCondition:  {"requires_relation": string(relation.TypeCP)},
				behavior.KeyPassive:    {"modifiers": []any{map[string]any{"key": "cp_badge", "value": "diamond"}}},
				behavior.KeyBindable:   {"bind_on_pickup": true},
			},
		},
		{
			ID: "ring_cp_gold", Category: model.CategoryCPRing, Name: "CP Gold Ring (30d)", Priority: 78, Rarity: 3,
			Behaviors: map[string]map[string]any{
				behavior.KeyEquippable: {"slot": string(model.SlotCPRing), "capacity": 1},
				behavior.KeyCondition:  {"requires_relation": string(relation.TypeCP)},
				behavior.KeyExpirable:  {"duration": "720h", "on_expire": behavior.ExpirePolicyRemove},
				behavior.KeyBindable:   {"bind_on_pickup": true},
			},
		},
	}
}

// All 汇聚全部九类模板，供 NewStack 一键装配演示/测试环境。
func All() []*model.ItemTemplate {
	var all []*model.ItemTemplate
	for _, group := range []func() []*model.ItemTemplate{
		Mounts, Badges, Headwear, VIPs, RelationCards, Banners, ChatBubbles, Nameplates, CPRings,
	} {
		all = append(all, group()...)
	}
	return all
}
