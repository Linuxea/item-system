// vip_test.go VIP：链式降级（月卡→周卡→vip0）继承目标时效，试用卡到期删除。
package item_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func vipStack(t *testing.T) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.VIPs()...)
	return inv, s, clock
}

func TestVIPDowngradeChain(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := vipStack(t)

	r, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "vip3_month", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := inv.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	if n, err := inv.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry: n=%d err=%v", n, err)
	}
	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.DefID != "vip1_week" || inst.Status != item.StatusEquipped {
		t.Fatalf("expected downgrade to vip1_week equipped, got %+v", inst)
	}
	if inst.ExpireAt == nil || !inst.ExpireAt.After(*clock) {
		t.Fatalf("downgraded vip should inherit vip1 duration: %+v", inst.ExpireAt)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	if n, err := inv.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry 2: n=%d err=%v", n, err)
	}
	inst, _ = s.Instances.Get(ctx, r.InstanceID)
	if inst.DefID != "vip0" || inst.Status != item.StatusEquipped || inst.ExpireAt != nil {
		t.Fatalf("expected final downgrade to permanent vip0, got %+v", inst)
	}

	if n, err := inv.RunExpiry(ctx, 10); err != nil || n != 0 {
		t.Fatalf("vip0 should never expire: n=%d err=%v", n, err)
	}
}

func TestVIPTrialRemoved(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := vipStack(t)

	r, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "vip_trial_1d", Count: 1})
	if err := inv.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	inv.RunExpiry(ctx, 10)

	if _, err := s.Instances.Get(ctx, r.InstanceID); err == nil {
		t.Fatalf("trial vip should be removed on expiry")
	}
	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotVIP]) != 0 {
		t.Fatalf("vip slot should be empty: %+v", snap.Slots[item.SlotVIP])
	}
}
