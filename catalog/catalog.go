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
