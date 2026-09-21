// equip.go 穿戴流程与展示快照：槽位容量、前置条件、按场景排序的快照构建。
// 穿戴能力由道具实现 Equippable 声明，前置条件由道具实现 EquipGated 自判。
package item

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Linuxea/item-system/event"
)

// 穿戴相关错误。
var (
	// ErrNotEquippable 道具未实现 Equippable。
	ErrNotEquippable = errors.New("item is not equippable")
	// ErrSlotFull 槽位容量已满。
	ErrSlotFull = errors.New("equip slot is full")
	// ErrAlreadyEquipped 实例已处于穿戴状态。
	ErrAlreadyEquipped = errors.New("item already equipped")
	// ErrNotEquipped 实例未穿戴。
	ErrNotEquipped = errors.New("item not equipped")
	// ErrConditionNotMet 穿戴前置条件不满足。
	ErrConditionNotMet = errors.New("equip condition not met")
)

// DisplayItem 快照中的展示条目：实例与道具定义信息的扁平组合，供展示层直接消费。
type DisplayItem struct {
	// InstanceID 实例 ID。
	InstanceID string
	// DefID 道具定义标识。
	DefID string
	// Name 展示名称。
	Name string
	// Priority 排序优先级，越大越靠前。
	Priority int
	// Rarity 稀有度。
	Rarity int
	// EquippedAt 穿戴时间（同级排序的兜底键）。
	EquippedAt time.Time
	// Modifiers 该道具的被动修饰符。
	Modifiers []Modifier
}

// Snapshot 展示快照：按槽位聚合的穿戴结果 + 全量修饰符。
// Version 为内容版本（各实例版本与记录数之和），供展示层做缓存失效。
type Snapshot struct {
	// Owner 玩家。
	Owner string
	// Version 内容版本，内容变化即变化。
	Version int64
	// Scene 场景名（chat/game 等）。
	Scene string
	// Slots 槽位 -> 排序后的展示条目。
	Slots map[Slot][]DisplayItem
	// Modifiers 全部已穿戴道具的修饰符汇总。
	Modifiers []Modifier
}

// SortPolicy 场景排序策略：输入候选条目，返回排序（可截断）后的条目。
type SortPolicy func(items []DisplayItem) []DisplayItem

// SortRegistry 场景 -> 排序策略注册表，未注册场景回落到 DefaultSort。
type SortRegistry struct {
	policies map[string]SortPolicy
}

// NewSortRegistry 创建排序注册表。
func NewSortRegistry() *SortRegistry {
	return &SortRegistry{policies: map[string]SortPolicy{}}
}

// Register 登记一个场景的排序策略。
func (r *SortRegistry) Register(scene string, p SortPolicy) {
	r.policies[scene] = p
}

// Policy 取场景策略；未注册时返回默认多级排序。
func (r *SortRegistry) Policy(scene string) SortPolicy {
	if p, ok := r.policies[scene]; ok {
		return p
	}
	return DefaultSort
}

// DefaultSort 默认多级排序：优先级降序 -> 稀有度降序 -> 穿戴时间升序。
// 返回新切片，不修改入参。
func DefaultSort(items []DisplayItem) []DisplayItem {
	out := append([]DisplayItem(nil), items...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}
		if a.Rarity != b.Rarity {
			return a.Rarity > b.Rarity
		}
		return a.EquippedAt.Before(b.EquippedAt)
	})
	return out
}

// TopN 组合器：在内层排序之上只保留前 n 条（如"只展示最好的 3 枚勋章"）。
func TopN(n int, inner SortPolicy) SortPolicy {
	return func(items []DisplayItem) []DisplayItem {
		sorted := inner(items)
		if len(sorted) > n {
			sorted = sorted[:n]
		}
		return sorted
	}
}

// Equip 穿戴实例：归属校验 -> 状态检查 -> 断言 Equippable
// -> 前置条件（EquipGated 道具自判）-> 槽位容量 -> 版本 CAS 更新状态
// -> 落穿戴记录 -> 发布事件。
func (inv *Inventory) Equip(ctx context.Context, owner, instanceID string) error {
	inst, err := inv.loadOwned(ctx, owner, instanceID)
	if err != nil {
		return err
	}
	if inst.Status == StatusEquipped {
		return ErrAlreadyEquipped
	}

	def, err := inv.Defs.Get(ctx, inst.DefID)
	if err != nil {
		return err
	}
	equippable, ok := def.(Equippable)
	if !ok {
		return ErrNotEquippable
	}
	// 前置条件由道具自己判断（等级/关系），库存层只问结果不代劳。
	if gated, ok := def.(EquipGated); ok {
		met, err := gated.CanEquip(ctx, inv, owner)
		if err != nil {
			return err
		}
		if !met {
			return ErrConditionNotMet
		}
	}

	slot := equippable.EquipSlot()
	capacity := equippable.Capacity()
	if capacity <= 0 {
		capacity = 1
	}
	records, err := inv.Equips.ListBySlot(ctx, owner, slot)
	if err != nil {
		return err
	}
	if len(records) >= capacity {
		return ErrSlotFull
	}

	// 先以乐观锁把实例置为已穿戴，再写穿戴记录。
	expect := inst.Version
	inst.Status = StatusEquipped
	inst.BumpVersion()
	if err := inv.Instances.Update(ctx, inst, expect); err != nil {
		return err
	}

	if err := inv.Equips.Save(ctx, EquipRecord{
		Owner:      owner,
		Slot:       slot,
		InstanceID: inst.ID,
		EquippedAt: inv.Now(),
	}); err != nil {
		return err
	}

	inv.Events.Publish(Equipped{
		Base:       event.Base{At: inv.Now()},
		Owner:      owner,
		DefID:      inst.DefID,
		InstanceID: inst.ID,
		Slot:       string(slot),
	})
	return nil
}

// Unequip 卸下实例：状态复位（版本 CAS）并删除穿戴记录；
// 记录缺失时仍算成功（容忍不一致残留）。
func (inv *Inventory) Unequip(ctx context.Context, owner, instanceID string) error {
	inst, err := inv.loadOwned(ctx, owner, instanceID)
	if err != nil {
		return err
	}
	if inst.Status != StatusEquipped {
		return ErrNotEquipped
	}

	records, err := inv.Equips.ListByOwner(ctx, owner)
	if err != nil {
		return err
	}
	var rec *EquipRecord
	for i := range records {
		if records[i].InstanceID == instanceID {
			rec = &records[i]
			break
		}
	}

	expect := inst.Version
	inst.Status = StatusNormal
	inst.BumpVersion()
	if err := inv.Instances.Update(ctx, inst, expect); err != nil {
		return err
	}

	if rec != nil {
		if err := inv.Equips.Delete(ctx, owner, rec.Slot, instanceID); err != nil {
			return err
		}
	}

	inv.Events.Publish(Unequipped{
		Base:       event.Base{At: inv.Now()},
		Owner:      owner,
		DefID:      inst.DefID,
		InstanceID: inst.ID,
		Slot:       slotOf(rec),
	})
	return nil
}

// Build 构建指定场景的展示快照：遍历穿戴记录，跳过失效条目
// （实例缺失/非穿戴态/已过期），按槽位分组，聚合 Passive 修饰符，
// 最后用该场景的 SortPolicy 对每个槽位排序（可截断）。
func (inv *Inventory) Build(ctx context.Context, owner, scene string) (*Snapshot, error) {
	records, err := inv.Equips.ListByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Owner: owner,
		Scene: scene,
		Slots: map[Slot][]DisplayItem{},
	}
	grouped := map[Slot][]DisplayItem{}

	for _, rec := range records {
		inst, err := inv.Instances.Get(ctx, rec.InstanceID)
		if err != nil || inst.Status != StatusEquipped || inst.Expired(inv.Now()) {
			continue
		}
		def, err := inv.Defs.Get(ctx, inst.DefID)
		if err != nil {
			continue
		}
		item := DisplayItem{
			InstanceID: inst.ID,
			DefID:      def.DefID(),
			Name:       def.DisplayName(),
			Priority:   def.SortPriority(),
			Rarity:     def.SortRarity(),
			EquippedAt: rec.EquippedAt,
		}
		if passive, ok := def.(Passive); ok {
			mods := passive.Modifiers()
			item.Modifiers = append([]Modifier(nil), mods...)
			snap.Modifiers = append(snap.Modifiers, mods...)
		}
		grouped[rec.Slot] = append(grouped[rec.Slot], item)
		snap.Version += inst.Version
	}

	policy := inv.Sorts.Policy(scene)
	for slot, items := range grouped {
		snap.Slots[slot] = policy(items)
	}
	snap.Version += int64(len(records))
	return snap, nil
}

// loadOwned 加载实例并校验归属。
func (inv *Inventory) loadOwned(ctx context.Context, owner, instanceID string) (*Instance, error) {
	inst, err := inv.Instances.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if inst.Owner != owner {
		return nil, ErrNotOwner
	}
	return inst, nil
}

// slotOf 从可空记录取槽位名，用于卸下事件兜底。
func slotOf(rec *EquipRecord) string {
	if rec == nil {
		return ""
	}
	return string(rec.Slot)
}
