// Package profile 提供穿戴领域服务与展示快照：槽位容量、前置条件、
// 按场景排序的快照构建。穿戴（EquipService）与查询（ProfileService）职责分离，
// 后者只依赖只读端口。
package profile

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/Linuxea/item-system/domain/behavior"
	"github.com/Linuxea/item-system/domain/event"
	"github.com/Linuxea/item-system/domain/model"
	"github.com/Linuxea/item-system/domain/repository"
)

var (
	// ErrInstanceNotFound 实例不存在。
	ErrInstanceNotFound = errors.New("instance not found")
	// ErrNotOwner 实例不属于该玩家。
	ErrNotOwner = errors.New("instance does not belong to owner")
	// ErrNotEquippable 道具不具备可穿戴行为。
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

// Clock 时钟端口，测试可注入固定时钟。
type Clock func() time.Time

// DisplayItem 快照中的展示条目：实例与模板信息的扁平组合，供展示层直接消费。
type DisplayItem struct {
	// InstanceID 实例 ID。
	InstanceID string
	// TemplateID 模板 ID。
	TemplateID string
	// Name 展示名称。
	Name string
	// Category 品类。
	Category model.Category
	// Priority 排序优先级，越大越靠前。
	Priority int
	// Rarity 稀有度。
	Rarity int
	// EquippedAt 穿戴时间（同级排序的兜底键）。
	EquippedAt time.Time
	// Modifiers 该道具的被动修饰符。
	Modifiers []model.Modifier
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
	Slots map[model.SlotType][]DisplayItem
	// Modifiers 全部已穿戴道具的修饰符汇总。
	Modifiers []model.Modifier
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
// 体现了排序策略的可组合设计。
func TopN(n int, inner SortPolicy) SortPolicy {
	return func(items []DisplayItem) []DisplayItem {
		sorted := inner(items)
		if len(sorted) > n {
			sorted = sorted[:n]
		}
		return sorted
	}
}

// instanceStore 消费侧窄接口：穿戴服务只使用实例仓储的读与条件更新。
type instanceStore interface {
	Get(ctx context.Context, id string) (*model.ItemInstance, error)
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
}

// equipRecords 消费侧窄接口：穿戴服务对穿戴记录仓储的使用切片。
type equipRecords interface {
	ListByOwner(ctx context.Context, owner string) ([]model.EquipRecord, error)
	ListBySlot(ctx context.Context, owner string, slot model.SlotType) ([]model.EquipRecord, error)
	Save(ctx context.Context, rec model.EquipRecord) error
	Delete(ctx context.Context, owner string, slot model.SlotType, instanceID string) error
}

// EquipService 穿戴领域服务：穿戴/卸下。
type EquipService struct {
	templates repository.TemplateSource
	instances instanceStore
	equips    equipRecords
	publisher event.Publisher
	compiler  behavior.Compiler
	now       Clock
}

// NewEquipService 构造穿戴服务，依赖全部经由参数注入。
func NewEquipService(
	templates repository.TemplateSource,
	instances instanceStore,
	equips equipRecords,
	publisher event.Publisher,
	compiler behavior.Compiler,
	now Clock,
) *EquipService {
	return &EquipService{
		templates: templates,
		instances: instances,
		equips:    equips,
		publisher: publisher,
		compiler:  compiler,
		now:       now,
	}
}

// Equip 穿戴实例：归属校验 -> 状态检查 -> 编译 -> 可穿戴组件
// -> 前置条件（组件自判）-> 槽位容量 -> 版本 CAS 更新状态 -> 落穿戴记录 -> 发布事件。
func (s *EquipService) Equip(ctx context.Context, owner, instanceID string) error {
	inst, err := s.loadOwned(ctx, owner, instanceID)
	if err != nil {
		return err
	}
	if inst.Status == model.StatusEquipped {
		return ErrAlreadyEquipped
	}

	tpl, err := s.templates.Get(ctx, inst.TemplateID)
	if err != nil {
		return err
	}
	compiled, err := s.compiler.Compile(tpl)
	if err != nil {
		return err
	}
	eq, ok := compiled.Equippable()
	if !ok {
		return ErrNotEquippable
	}
	// 条件由 Condition 组件自己判断（等级/关系），服务只问结果不代劳。
	if cond, ok := compiled.Condition(); ok {
		met, err := cond.Met(ctx, owner)
		if err != nil {
			return err
		}
		if !met {
			return ErrConditionNotMet
		}
	}

	records, err := s.equips.ListBySlot(ctx, owner, eq.Slot)
	if err != nil {
		return err
	}
	if len(records) >= eq.Capacity {
		return ErrSlotFull
	}

	// 先以乐观锁把实例置为已穿戴，再写穿戴记录。
	expect := inst.Version
	inst.Status = model.StatusEquipped
	inst.BumpVersion()
	if err := s.instances.Update(ctx, inst, expect); err != nil {
		return err
	}

	if err := s.equips.Save(ctx, model.EquipRecord{
		Owner:      owner,
		Slot:       eq.Slot,
		InstanceID: inst.ID,
		EquippedAt: s.now(),
	}); err != nil {
		return err
	}

	s.publisher.Publish(event.Equipped{
		Base:       event.Base{At: s.now()},
		Owner:      owner,
		TemplateID: inst.TemplateID,
		InstanceID: inst.ID,
		Slot:       string(eq.Slot),
	})
	return nil
}

// Unequip 卸下实例：状态复位（版本 CAS）并删除穿戴记录；记录缺失时仍算成功（容忍不一致残留）。
func (s *EquipService) Unequip(ctx context.Context, owner, instanceID string) error {
	inst, err := s.loadOwned(ctx, owner, instanceID)
	if err != nil {
		return err
	}
	if inst.Status != model.StatusEquipped {
		return ErrNotEquipped
	}

	records, err := s.equips.ListByOwner(ctx, owner)
	if err != nil {
		return err
	}
	var rec *model.EquipRecord
	for i := range records {
		if records[i].InstanceID == instanceID {
			rec = &records[i]
			break
		}
	}

	expect := inst.Version
	inst.Status = model.StatusNormal
	inst.BumpVersion()
	if err := s.instances.Update(ctx, inst, expect); err != nil {
		return err
	}

	if rec != nil {
		if err := s.equips.Delete(ctx, owner, rec.Slot, instanceID); err != nil {
			return err
		}
	}

	s.publisher.Publish(event.Unequipped{
		Base:       event.Base{At: s.now()},
		Owner:      owner,
		TemplateID: inst.TemplateID,
		InstanceID: inst.ID,
		Slot:       slotOf(rec),
	})
	return nil
}

// loadOwned 加载实例并校验归属。
func (s *EquipService) loadOwned(ctx context.Context, owner, instanceID string) (*model.ItemInstance, error) {
	inst, err := s.instances.Get(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	if inst.Owner != owner {
		return nil, ErrNotOwner
	}
	return inst, nil
}

// instanceReader 消费侧窄接口（只读）：快照服务连写方法都不可见。
type instanceReader interface {
	Get(ctx context.Context, id string) (*model.ItemInstance, error)
}

// equipHistory 消费侧窄接口（只读）：快照服务只需按玩家列穿戴记录。
type equipHistory interface {
	ListByOwner(ctx context.Context, owner string) ([]model.EquipRecord, error)
}

// ProfileService 展示快照领域服务：只读聚合，无任何写端口。
type ProfileService struct {
	instances instanceReader
	equips    equipHistory
	templates repository.TemplateSource
	compiler  behavior.Compiler
	sorts     *SortRegistry
	now       Clock
}

// NewProfileService 构造快照服务，依赖全部经由参数注入。
func NewProfileService(
	instances instanceReader,
	equips equipHistory,
	templates repository.TemplateSource,
	compiler behavior.Compiler,
	sorts *SortRegistry,
	now Clock,
) *ProfileService {
	return &ProfileService{
		instances: instances,
		equips:    equips,
		templates: templates,
		compiler:  compiler,
		sorts:     sorts,
		now:       now,
	}
}

// Build 构建指定场景的展示快照：遍历穿戴记录，跳过失效条目
// （实例缺失/非穿戴态/已过期），按槽位分组，聚合 Passive 修饰符，
// 最后用该场景的 SortPolicy 对每个槽位排序（可截断）。
func (s *ProfileService) Build(ctx context.Context, owner, scene string) (*Snapshot, error) {
	records, err := s.equips.ListByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{
		Owner: owner,
		Scene: scene,
		Slots: map[model.SlotType][]DisplayItem{},
	}
	grouped := map[model.SlotType][]DisplayItem{}

	for _, rec := range records {
		inst, err := s.instances.Get(ctx, rec.InstanceID)
		if err != nil || inst.Status != model.StatusEquipped || inst.Expired(s.now()) {
			continue
		}
		tpl, err := s.templates.Get(ctx, inst.TemplateID)
		if err != nil {
			continue
		}
		item := DisplayItem{
			InstanceID: inst.ID,
			TemplateID: tpl.ID,
			Name:       tpl.Name,
			Category:   tpl.Category,
			Priority:   tpl.Priority,
			Rarity:     tpl.Rarity,
			EquippedAt: rec.EquippedAt,
		}
		if compiled, err := s.compiler.Compile(tpl); err == nil {
			if passive, ok := compiled.Passive(); ok {
				item.Modifiers = append([]model.Modifier(nil), passive.Modifiers...)
				snap.Modifiers = append(snap.Modifiers, passive.Modifiers...)
			}
		}
		grouped[rec.Slot] = append(grouped[rec.Slot], item)
		snap.Version += inst.Version
	}

	policy := s.sorts.Policy(scene)
	for slot, items := range grouped {
		snap.Slots[slot] = policy(items)
	}
	snap.Version += int64(len(records))
	return snap, nil
}

// slotOf 从可空记录取槽位名，用于卸下事件兜底。
func slotOf(rec *model.EquipRecord) string {
	if rec == nil {
		return ""
	}
	return string(rec.Slot)
}
