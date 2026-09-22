package engine

import (
	"context"
	"sort"
	"time"

	"github.com/Linuxea/item-system/item"
)

// Entry 展示快照中的一件已穿戴道具。
type Entry struct {
	// InstanceID 实例 ID。
	InstanceID string
	// ItemID 道具 ID。
	ItemID string
	// Name 展示名称。
	Name string
	// Priority 展示优先级。
	Priority int
	// Rarity 稀有度。
	Rarity int
	// EquippedAt 穿戴时间。
	EquippedAt time.Time
}

// Snapshot 玩家在某个场景下的展示快照。
type Snapshot struct {
	// Owner 玩家。
	Owner string
	// Scene 场景名。
	Scene string
	// Slots 按槽位聚合的已穿戴道具，槽位内已按该场景的排序策略排好。
	Slots map[item.Slot][]Entry
	// Modifiers 全部已穿戴道具的被动属性合并结果；同名键后者覆盖前者。
	Modifiers map[string]any
}

// Comparator 两件道具的先后比较：返回负数表示 a 排在 b 前，正数表示在后，0 表示分不出。
type Comparator func(a, b Entry) int

// SortPolicy 场景排序策略：按顺序应用多个比较器，前一个分不出胜负时交给下一个。
type SortPolicy []Comparator

// ByPriorityDesc 优先级高的排前面。
func ByPriorityDesc(a, b Entry) int { return b.Priority - a.Priority }

// ByRarityDesc 稀有度高的排前面。
func ByRarityDesc(a, b Entry) int { return b.Rarity - a.Rarity }

// ByRecentlyEquipped 最近穿戴的排前面。
func ByRecentlyEquipped(a, b Entry) int {
	switch {
	case a.EquippedAt.After(b.EquippedAt):
		return -1
	case a.EquippedAt.Before(b.EquippedAt):
		return 1
	default:
		return 0
	}
}

// ByName 按名称字典序，作为最后的稳定兜底。
func ByName(a, b Entry) int {
	switch {
	case a.Name < b.Name:
		return -1
	case a.Name > b.Name:
		return 1
	default:
		return 0
	}
}

// DefaultSortPolicy 未为场景单独配置时使用的默认策略。
var DefaultSortPolicy = SortPolicy{ByPriorityDesc, ByRarityDesc, ByRecentlyEquipped, ByName}

// BuildProfile 构建玩家在指定场景下的展示快照。
// 场景未配置排序策略时使用 DefaultSortPolicy；已过期的实例不会进入快照。
func (e *Engine) BuildProfile(ctx context.Context, owner, scene string) (*Snapshot, error) {
	owned, err := e.store.ListByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	now := e.now()

	snap := &Snapshot{
		Owner:     owner,
		Scene:     scene,
		Slots:     map[item.Slot][]Entry{},
		Modifiers: map[string]any{},
	}

	for _, inst := range owned {
		if !inst.Equipped || inst.Expired(now) {
			continue
		}
		it, ok := e.items.Lookup(inst.ItemID)
		if !ok {
			continue
		}
		slot, ok := item.SlotOf(it)
		if !ok {
			continue
		}
		snap.Slots[slot] = append(snap.Slots[slot], Entry{
			InstanceID: inst.ID,
			ItemID:     inst.ItemID,
			Name:       it.Name(),
			Priority:   item.PriorityOf(it),
			Rarity:     item.RarityOf(it),
			EquippedAt: inst.EquippedAt,
		})
		// 问：你有被动属性吗？
		if p, ok := it.(item.Passive); ok {
			for _, m := range p.Modifiers() {
				snap.Modifiers[m.Key] = m.Value
			}
		}
	}

	policy := e.sceneSort(scene)
	for slot := range snap.Slots {
		entries := snap.Slots[slot]
		sort.SliceStable(entries, func(i, j int) bool {
			for _, cmp := range policy {
				if c := cmp(entries[i], entries[j]); c != 0 {
					return c < 0
				}
			}
			return false
		})
		snap.Slots[slot] = entries
	}
	return snap, nil
}

// sceneSort 取场景对应的排序策略。
func (e *Engine) sceneSort(scene string) SortPolicy {
	if p, ok := e.scenes[scene]; ok && len(p) > 0 {
		return p
	}
	return DefaultSortPolicy
}
