// Package expiry 提供道具过期领域服务：按模板策略批量处理已过期实例
// （删除 / 卸下保留 / 降级）。
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

// ErrUnknownTemplate 过期实例的模板已不存在（无法执行策略）。
var ErrUnknownTemplate = errors.New("unknown template")

// Clock 时钟端口，测试可注入固定时钟。
type Clock func() time.Time

// instanceStore 消费侧窄接口：过期服务只使用实例仓储的写方法与过期查询。
type instanceStore interface {
	Update(ctx context.Context, inst *model.ItemInstance, expectVersion int64) error
	Delete(ctx context.Context, id string) error
	ListExpired(ctx context.Context, before time.Time, limit int) ([]*model.ItemInstance, error)
}

// equipStore 消费侧窄接口：处理过期时清理/维护穿戴记录。
type equipStore interface {
	ListByOwner(ctx context.Context, owner string) ([]model.EquipRecord, error)
	DeleteByInstance(ctx context.Context, instanceID string) error
}

// Service 过期领域服务。
type Service struct {
	templates repository.TemplateSource
	instances instanceStore
	equips    equipStore
	publisher event.Publisher
	compiler  behavior.Compiler
	now       Clock
}

// NewService 构造过期服务，依赖全部经由参数注入。
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

// Run 批量处理一批过期实例，limit 控制单批规模（供定时任务分批消费）。
// 模板已缺失的实例跳过（计数不含），其余错误立即中止并返回已处理数。
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

// applyPolicy 按模板配置执行过期策略：
// 无 Expirable 组件或策略未识别 -> 删除；downgrade -> 降级；unequip -> 卸下保留。
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

// remove 删除实例及其穿戴记录，并发过期事件。
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

// unequipAndKeep 仅卸下并保留实例：清穿戴记录，把状态复位为正常（版本 CAS）。
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

// downgrade 把实例降级为 to 模板：时效改为目标模板自身的时效（支持 vip3→vip1→vip0 链式降级）；
// 若降级前处于穿戴态且目标槽位一致，则保持穿戴，否则仅卸下。
// 原实例被原地改写（同 ID 换模板），避免"删旧发新"造成的穿戴断裂。
func (s *Service) downgrade(ctx context.Context, inst *model.ItemInstance, compiled *behavior.Compiled, to string) error {
	target, err := s.templates.Get(ctx, to)
	if err != nil {
		return err
	}
	targetCompiled, err := s.compiler.Compile(target)
	if err != nil {
		return err
	}

	// 记录降级前的穿戴状态与槽位，供"保持穿戴"判断。
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
	// 降级目标的时效继承目标模板：目标可过期则重新计时，否则变为永久。
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

// slotOf 取编译产物中的穿戴槽位；目标不可穿戴时 ok 为 false。
func (s *Service) slotOf(compiled *behavior.Compiled) (model.SlotType, bool) {
	eq, ok := compiled.Equippable()
	if !ok {
		return "", false
	}
	return eq.Slot, true
}

// publish 发布过期事件。
func (s *Service) publish(inst *model.ItemInstance, policy string) {
	s.publisher.Publish(event.Expired{
		Base:       event.Base{At: s.now()},
		Owner:      inst.Owner,
		TemplateID: inst.TemplateID,
		InstanceID: inst.ID,
		Policy:     policy,
	})
}
