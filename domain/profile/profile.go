package profile

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

var (
	ErrInstanceNotFound = errors.New("instance not found")
	ErrNotOwner         = errors.New("instance does not belong to owner")
	ErrNotEquippable    = errors.New("item is not equippable")
	ErrSlotFull         = errors.New("equip slot is full")
	ErrAlreadyEquipped  = errors.New("item already equipped")
	ErrNotEquipped      = errors.New("item not equipped")
	ErrConditionNotMet  = errors.New("equip condition not met")
)

type Clock func() time.Time

type DisplayItem struct {
	InstanceID string
	TemplateID string
	Name       string
	Category   model.Category
	Priority   int
	Rarity     int
	EquippedAt time.Time
	Modifiers  []model.Modifier
}

type Snapshot struct {
	Owner     string
	Version   int64
	Scene     string
	Slots     map[model.SlotType][]DisplayItem
	Modifiers []model.Modifier
}

type SortPolicy func(items []DisplayItem) []DisplayItem

type SortRegistry struct {
	policies map[string]SortPolicy
}

func NewSortRegistry() *SortRegistry {
	return &SortRegistry{policies: map[string]SortPolicy{}}
}

func (r *SortRegistry) Register(scene string, p SortPolicy) {
	r.policies[scene] = p
}

func (r *SortRegistry) Policy(scene string) SortPolicy {
	if p, ok := r.policies[scene]; ok {
		return p
	}
	return DefaultSort
}

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

type EquipService struct {
	templates  repository.TemplateSource
	instances  repository.InstanceRepo
	equips     repository.EquipRepo
	publisher  event.Publisher
	registry   *behavior.Registry
	conditions behavior.ConditionChecker
	now        Clock
}

func NewEquipService(
	templates repository.TemplateSource,
	instances repository.InstanceRepo,
	equips repository.EquipRepo,
	publisher event.Publisher,
	registry *behavior.Registry,
	conditions behavior.ConditionChecker,
	now Clock,
) *EquipService {
	return &EquipService{
		templates:  templates,
		instances:  instances,
		equips:     equips,
		publisher:  publisher,
		registry:   registry,
		conditions: conditions,
		now:        now,
	}
}

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
	compiled, err := s.registry.Compile(tpl)
	if err != nil {
		return err
	}
	eq, ok := compiled.Equippable()
	if !ok {
		return ErrNotEquippable
	}
	if cond, ok := compiled.Condition(); ok && s.conditions != nil {
		met, err := s.conditions.Satisfied(ctx, owner, cond)
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

type ProfileService struct {
	instances repository.InstanceRepo
	equips    repository.EquipRepo
	templates repository.TemplateSource
	registry  *behavior.Registry
	sorts     *SortRegistry
	now       Clock
}

func NewProfileService(
	instances repository.InstanceRepo,
	equips repository.EquipRepo,
	templates repository.TemplateSource,
	registry *behavior.Registry,
	sorts *SortRegistry,
	now Clock,
) *ProfileService {
	return &ProfileService{
		instances: instances,
		equips:    equips,
		templates: templates,
		registry:  registry,
		sorts:     sorts,
		now:       now,
	}
}

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
		if compiled, err := s.registry.Compile(tpl); err == nil {
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

func slotOf(rec *model.EquipRecord) string {
	if rec == nil {
		return ""
	}
	return string(rec.Slot)
}
