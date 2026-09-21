// mount_test.go 座驾：等级前置、座驾槽独占、限时款到期删除。
package item_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

// mountStack 装配座驾定义（等级源来自栈组件）。
func mountStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.Mounts(s.Levels)...)
	return inv, s, clock
}

func TestMountEquipRequiresLevel(t *testing.T) {
	ctx := context.Background()
	inv, s, _ := mountStack(t)

	r, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "mount_dragon", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := inv.Equip(ctx, "p1", r.InstanceID); !errors.Is(err, item.ErrConditionNotMet) {
		t.Fatalf("expected ErrConditionNotMet for low level, got %v", err)
	}

	s.Levels.SetLevel("p1", 30)
	if err := inv.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip at level 30: %v", err)
	}

	snap, err := inv.Build(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	mounts := snap.Slots[item.SlotMount]
	if len(mounts) != 1 || mounts[0].DefID != "mount_dragon" {
		t.Fatalf("mount slot wrong: %+v", mounts)
	}
	found := false
	for _, m := range snap.Modifiers {
		if m.Key == "move_speed" && m.Value == int64(150) {
			found = true
		}
	}
	if !found {
		t.Fatalf("move_speed modifier missing: %+v", snap.Modifiers)
	}
}

func TestMountSingleSlotExclusive(t *testing.T) {
	ctx := context.Background()
	inv, s, _ := mountStack(t)
	s.Levels.SetLevel("p1", 99)

	dragon, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "mount_dragon", Count: 1})
	cloud, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "mount_cloud", Count: 1})
	if err := inv.Equip(ctx, "p1", dragon.InstanceID); err != nil {
		t.Fatalf("equip dragon: %v", err)
	}
	if err := inv.Equip(ctx, "p1", cloud.InstanceID); !errors.Is(err, item.ErrSlotFull) {
		t.Fatalf("expected ErrSlotFull, got %v", err)
	}
	if err := inv.Unequip(ctx, "p1", dragon.InstanceID); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	if err := inv.Equip(ctx, "p1", cloud.InstanceID); err != nil {
		t.Fatalf("equip cloud after unequip: %v", err)
	}
}

func TestMountExpirableRemoved(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := mountStack(t)
	s.Levels.SetLevel("p1", 20)

	r, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "mount_ember_7d", Count: 1})
	if err := inv.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	n, err := inv.RunExpiry(ctx, 10)
	if err != nil || n != 1 {
		t.Fatalf("expiry run: n=%d err=%v", n, err)
	}
	if _, err := s.Instances.Get(ctx, r.InstanceID); err == nil {
		t.Fatalf("expired mount should be removed")
	}
	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotMount]) != 0 {
		t.Fatalf("mount slot should be empty: %+v", snap.Slots[item.SlotMount])
	}
}
