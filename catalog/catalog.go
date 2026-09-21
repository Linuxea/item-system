package catalog

import (
	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/model"
)

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
