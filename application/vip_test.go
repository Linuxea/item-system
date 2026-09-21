package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/catalog"
	"github.com/Linuxea/item-system/domain/grant"
	"github.com/Linuxea/item-system/domain/model"
)

func TestVIPDowngradeChain(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.VIPs())

	r, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "vip3_month", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := app.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	if n, err := app.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry: n=%d err=%v", n, err)
	}
	inst, _ := s.Instances.Get(ctx, r.InstanceID)
	if inst.TemplateID != "vip1_week" || inst.Status != model.StatusEquipped {
		t.Fatalf("expected downgrade to vip1_week equipped, got %+v", inst)
	}
	if inst.ExpireAt == nil || !inst.ExpireAt.After(*clock) {
		t.Fatalf("downgraded vip should inherit vip1 duration: %+v", inst.ExpireAt)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	if n, err := app.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry 2: n=%d err=%v", n, err)
	}
	inst, _ = s.Instances.Get(ctx, r.InstanceID)
	if inst.TemplateID != "vip0" || inst.Status != model.StatusEquipped || inst.ExpireAt != nil {
		t.Fatalf("expected final downgrade to permanent vip0, got %+v", inst)
	}

	if n, err := app.RunExpiry(ctx, 10); err != nil || n != 0 {
		t.Fatalf("vip0 should never expire: n=%d err=%v", n, err)
	}
}

func TestVIPTrialRemoved(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.VIPs())

	r, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "vip_trial_1d", Count: 1})
	if err := app.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	app.RunExpiry(ctx, 10)

	if _, err := s.Instances.Get(ctx, r.InstanceID); err == nil {
		t.Fatalf("trial vip should be removed on expiry")
	}
	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotVIP]) != 0 {
		t.Fatalf("vip slot should be empty: %+v", snap.Slots[model.SlotVIP])
	}
}
