package engine

import (
	"context"
	"fmt"

	"github.com/Linuxea/item-system/event"
	"github.com/Linuxea/item-system/item"
)

// Equip 穿戴道具。
//
// 顺序：归属与时效校验 -> 询问是否可穿戴 -> 让道具自己判断前置条件 -> 槽位容量校验 -> CAS 写入。
// 槽位已满时返回 ErrSlotFull，由调用方决定先卸下哪一件（引擎不替玩家做这个决定）。
// 重复穿戴同一实例是幂等的，直接返回 nil。
func (e *Engine) Equip(ctx context.Context, owner, instanceID string) error {
	inst, err := e.mustOwn(ctx, owner, instanceID)
	if err != nil {
		return err
	}
	if inst.Equipped {
		return nil
	}
	if inst.Expired(e.now()) {
		return item.ErrExpired
	}
	it, err := e.lookup(inst.ItemID)
	if err != nil {
		return err
	}

	// 问：你能穿吗？穿哪个槽？那个槽能同时穿几件？
	eq, ok := it.(item.Equippable)
	if !ok {
		return fmt.Errorf("%w: %s", item.ErrNotEquippable, it.Name())
	}

	// 问：你有前置条件吗？有就你自己判断，引擎不代劳。
	if c, ok := it.(item.Conditional); ok {
		if err := c.CanEquip(ctx, owner); err != nil {
			return err
		}
	}

	used, err := e.countEquippedInSlot(ctx, owner, eq.Slot(), instanceID)
	if err != nil {
		return err
	}
	if used >= eq.Capacity() {
		return fmt.Errorf("%w: %s 槽 %d/%d", item.ErrSlotFull, eq.Slot(), used, eq.Capacity())
	}

	expected := inst.Version
	inst.Equipped = true
	inst.EquippedAt = e.now()
	inst.Version++
	if err := e.store.Update(ctx, inst, expected); err != nil {
		return err
	}

	e.publish(ctx, event.KindEquipped, inst, 1, map[string]any{"slot": string(eq.Slot())})
	return nil
}

// Unequip 卸下道具。未在穿戴中时返回 ErrNotEquipped。
func (e *Engine) Unequip(ctx context.Context, owner, instanceID string) error {
	inst, err := e.mustOwn(ctx, owner, instanceID)
	if err != nil {
		return err
	}
	if !inst.Equipped {
		return item.ErrNotEquipped
	}
	return e.unequip(ctx, inst)
}

// unequip 内部卸下：过期处置、关系解除联动也会调用它。
func (e *Engine) unequip(ctx context.Context, inst *item.Instance) error {
	expected := inst.Version
	inst.Equipped = false
	inst.Version++
	if err := e.store.Update(ctx, inst, expected); err != nil {
		return err
	}
	e.publish(ctx, event.KindUnequipped, inst, 1, nil)
	return nil
}

// UnequipSlot 卸下玩家在指定槽位穿戴的全部道具，返回被卸下的实例 ID。
// 关系解除时用它联动卸下关系卡、CP 戒指。
func (e *Engine) UnequipSlot(ctx context.Context, owner string, slot item.Slot) ([]string, error) {
	owned, err := e.store.ListByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, inst := range owned {
		if !inst.Equipped {
			continue
		}
		it, ok := e.items.Lookup(inst.ItemID)
		if !ok {
			continue
		}
		if s, ok := item.SlotOf(it); !ok || s != slot {
			continue
		}
		if err := e.unequip(ctx, inst); err != nil {
			return removed, err
		}
		removed = append(removed, inst.ID)
	}
	return removed, nil
}

// countEquippedInSlot 统计玩家在指定槽位已穿戴的件数，排除 exclude 指定的实例。
func (e *Engine) countEquippedInSlot(ctx context.Context, owner string, slot item.Slot, exclude string) (int, error) {
	owned, err := e.store.ListByOwner(ctx, owner)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, other := range owned {
		if !other.Equipped || other.ID == exclude {
			continue
		}
		it, ok := e.items.Lookup(other.ItemID)
		if !ok {
			continue
		}
		if s, ok := item.SlotOf(it); ok && s == slot {
			n++
		}
	}
	return n, nil
}
