// expiry.go 过期批处理：按道具声明的 Expirable 策略处理已过期实例
// （删除 / 卸下保留 / 降级）。策略由道具声明，执行在通用层。
package item

import (
	"context"
	"errors"

	"github.com/Linuxea/item-system/event"
)

// ErrUnknownDef 过期实例的道具定义已不存在（无法执行策略）。
var ErrUnknownDef = errors.New("unknown item def")

// RunExpiry 批量处理一批过期实例，limit 控制单批规模（供定时任务分批消费）。
// 定义已缺失的实例跳过（计数不含），其余错误立即中止并返回已处理数。
func (inv *Inventory) RunExpiry(ctx context.Context, limit int) (int, error) {
	expired, err := inv.Instances.ListExpired(ctx, inv.Now(), limit)
	if err != nil {
		return 0, err
	}

	processed := 0
	for _, inst := range expired {
		if err := inv.applyExpirePolicy(ctx, inst); err != nil {
			if errors.Is(err, ErrUnknownDef) {
				continue
			}
			return processed, err
		}
		processed++
	}
	return processed, nil
}

// applyExpirePolicy 按道具声明的策略执行过期处理：
// 未实现 Expirable 或策略未识别 -> 删除；downgrade -> 降级；unequip -> 卸下保留。
func (inv *Inventory) applyExpirePolicy(ctx context.Context, inst *Instance) error {
	def, err := inv.Defs.Get(ctx, inst.DefID)
	if err != nil {
		return ErrUnknownDef
	}
	expirable, ok := def.(Expirable)
	if !ok {
		return inv.removeExpired(ctx, inst)
	}

	switch expirable.ExpirePolicy() {
	case PolicyDowngrade:
		if expirable.DowngradeTo() == "" {
			return inv.removeExpired(ctx, inst)
		}
		return inv.downgrade(ctx, inst, expirable.DowngradeTo())
	case PolicyUnequip:
		return inv.unequipExpired(ctx, inst)
	default:
		return inv.removeExpired(ctx, inst)
	}
}

// removeExpired 删除实例及其穿戴记录，并发过期事件。
func (inv *Inventory) removeExpired(ctx context.Context, inst *Instance) error {
	if err := inv.Equips.DeleteByInstance(ctx, inst.ID); err != nil {
		return err
	}
	if err := inv.Instances.Delete(ctx, inst.ID); err != nil {
		return err
	}
	inv.publishExpired(inst, PolicyRemove)
	return nil
}

// unequipExpired 仅卸下并保留实例：清穿戴记录，把状态复位为正常（版本 CAS）。
func (inv *Inventory) unequipExpired(ctx context.Context, inst *Instance) error {
	if err := inv.Equips.DeleteByInstance(ctx, inst.ID); err != nil {
		return err
	}
	if inst.Status == StatusEquipped {
		expect := inst.Version
		inst.Status = StatusNormal
		inst.BumpVersion()
		if err := inv.Instances.Update(ctx, inst, expect); err != nil {
			return err
		}
	}
	inv.publishExpired(inst, PolicyUnequip)
	return nil
}

// downgrade 把实例降级为 to 定义：时效改为目标道具自身的时效
// （支持 vip3→vip1→vip0 链式降级）；若降级前处于穿戴态且目标槽位一致，
// 则保持穿戴，否则仅卸下。原实例被原地改写（同 ID 换定义），避免
// "删旧发新"造成的穿戴断裂。
func (inv *Inventory) downgrade(ctx context.Context, inst *Instance, to string) error {
	target, err := inv.Defs.Get(ctx, to)
	if err != nil {
		return err
	}

	// 记录降级前的穿戴状态与槽位，供"保持穿戴"判断。
	wasEquipped := inst.Status == StatusEquipped
	var currentSlot Slot
	if wasEquipped {
		records, err := inv.Equips.ListByOwner(ctx, inst.Owner)
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

	targetSlot, keepEquipped := equipSlotOf(target)
	if wasEquipped && (!keepEquipped || targetSlot != currentSlot) {
		if err := inv.Equips.DeleteByInstance(ctx, inst.ID); err != nil {
			return err
		}
		wasEquipped = false
	}

	expect := inst.Version
	inst.DefID = to
	inst.Status = StatusNormal
	if wasEquipped && keepEquipped && targetSlot == currentSlot {
		inst.Status = StatusEquipped
	}
	// 降级目标的时效继承目标道具：目标可过期则重新计时，否则变为永久。
	if exp, ok := target.(Expirable); ok && exp.Lifetime() > 0 {
		t := inv.Now().Add(exp.Lifetime())
		inst.ExpireAt = &t
	} else {
		inst.ExpireAt = nil
	}
	inst.BumpVersion()
	if err := inv.Instances.Update(ctx, inst, expect); err != nil {
		return err
	}
	inv.publishExpired(inst, PolicyDowngrade)
	return nil
}

// equipSlotOf 取道具声明的穿戴槽位；未实现 Equippable 时 ok 为 false。
func equipSlotOf(def Def) (Slot, bool) {
	equippable, ok := def.(Equippable)
	if !ok {
		return "", false
	}
	return equippable.EquipSlot(), true
}

// publishExpired 发布过期事件。
func (inv *Inventory) publishExpired(inst *Instance, policy string) {
	inv.Events.Publish(Expired{
		Base:       event.Base{At: inv.Now()},
		Owner:      inst.Owner,
		DefID:      inst.DefID,
		InstanceID: inst.ID,
		Policy:     policy,
	})
}
