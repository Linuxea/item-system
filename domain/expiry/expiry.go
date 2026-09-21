package expiry

import (
	"context"
	"errors"
	"time"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/event"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/repository"
)

var ErrUnknownTemplate = errors.New("unknown template")

type Clock func() time.Time

type instanceStore interface {
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
	Delete(ctx context.Context, id string) error
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*model.ItemInstance, error)
}

type equipStore interface {
	ListByOwner(ctx context.Context, owner string) ([]model.EquipRecord, error)
	DeleteByInstance(ctx context.Context, instanceID string) error
}

type Service struct {
	templates repository.TemplateSource
	instances instanceStore
	equips    equipStore
	publisher event.Publisher
	compiler  behavior.Compiler
	now       Clock
}

func NewService(
	templates repository.TemplateSource,
	instances instanceStore,
	equips equipStore,
	publisher event.Publisher,
	compiler behavior.Compiler,
	now Clock,
) *Service {
	return &Service{
		templates: templates,
		instances: instances,
		equips:    equips,
		publisher: publisher,
		compiler:  compiler,
		now:       now,
	}
}

func (s *Service) Run(ctx context.Context, limit int) (int, error) {
	expired, err := s.instances.ListExpired(ctx, s.now(), limit)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, inst := range expired {
		if err := s.applyPolicy(ctx, inst); err != nil {
			if errors.Is(err, ErrUnknownTemplate) {
				continue
			}
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (s *Service) applyPolicy(ctx context.Context, inst *model.ItemInstance) error {
	tpl, err := s.templates.Get(ctx, inst.TemplateID)
	if err != nil {
		return ErrUnknownTemplate
	}
	compiled, err := s.compiler.Compile(tpl)
	if err != nil {
		return err
	}
	expirable, ok := compiled.Expirable()
	if !ok {
		return s.remove(ctx, inst)
	}

	switch expirable.OnExpire {
	case behavior.ExpirePolicyDowngrade:
		if expirable.DowngradeTo == "" {
			return s.remove(ctx, inst)
		}
		return s.downgrade(ctx, inst, compiled, expirable.DowngradeTo)
	case behavior.ExpirePolicyUnequip:
		return s.unequipAndKeep(ctx, inst)
	default:
		return s.remove(ctx, inst)
	}
}

func (s *Service) remove(ctx context.Context, inst *model.ItemInstance) error {
	if err := s.equips.DeleteByInstance(ctx, inst.ID); err != nil {
		return err
	}
	if err := s.instances.Delete(ctx, inst.ID); err != nil {
		return err
	}
	s.publish(inst, behavior.ExpirePolicyRemove)
	return nil
}

func (s *Service) unequipAndKeep(ctx context.Context, inst *model.ItemInstance) error {
	if err := s.equips.DeleteByInstance(ctx, inst.ID); err != nil {
		return err
	}
	if inst.Status == model.StatusEquipped {
		expect := inst.Version
		inst.Status = model.StatusNormal
		inst.BumpVersion()
		if err := s.instances.Update(ctx, inst, expect); err != nil {
			return err
		}
	}
	s.publish(inst, behavior.ExpirePolicyUnequip)
	return nil
}

func (s *Service) downgrade(ctx context.Context, inst *model.ItemInstance, compiled *behavior.Compiled, to string) error {
	target, err := s.templates.Get(ctx, to)
	if err != nil {
		return err
	}
	targetCompiled, err := s.compiler.Compile(target)
	if err != nil {
		return err
	}

	wasEquipped := inst.Status == model.StatusEquipped
	var currentSlot model.SlotType
	if wasEquipped {
		records, err := s.equips.ListByOwner(ctx, inst.Owner)
		if err != nil {
			return err
		}
		for _, rec := range records {
			if rec.InstanceID == inst.ID {
				currentSlot = rec.Slot
				break
			}
		}
	}

	targetSlot, keepEquipped := s.slotOf(targetCompiled)
	if wasEquipped && (!keepEquipped || targetSlot != currentSlot) {
		if err := s.equips.DeleteByInstance(ctx, inst.ID); err != nil {
			return err
		}
		wasEquipped = false
	}

	expect := inst.Version
	inst.TemplateID = to
	inst.Status = model.StatusNormal
	if wasEquipped && keepEquipped && targetSlot == currentSlot {
		inst.Status = model.StatusEquipped
	}
	if targetExpirable, ok := targetCompiled.Expirable(); ok && targetExpirable.Duration > 0 {
		t := s.now().Add(targetExpirable.Duration)
		inst.ExpireAt = &t
	} else {
		inst.ExpireAt = nil
	}
	inst.BumpVersion()
	if err := s.instances.Update(ctx, inst, expect); err != nil {
		return err
	}
	s.publish(inst, behavior.ExpirePolicyDowngrade)
	return nil
}

func (s *Service) slotOf(compiled *behavior.Compiled) (model.SlotType, bool) {
	eq, ok := compiled.Equippable()
	if !ok {
		return "", false
	}
	return eq.Slot, true
}

func (s *Service) publish(inst *model.ItemInstance, policy string) {
	s.publisher.Publish(event.Expired{
		Base:       event.Base{At: s.now()},
		Owner:      inst.Owner,
		TemplateID: inst.TemplateID,
		InstanceID: inst.ID,
		Policy:     policy,
	})
}
