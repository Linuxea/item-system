package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/linuxea/item-system/application"
	"github.com/linuxea/item-system/catalog"
	"github.com/linuxea/item-system/domain/grant"
	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/profile"
	"github.com/linuxea/item-system/infrastructure/memory"
)

func newStackWithClock(t *testing.T, tpls []*model.ItemTemplate) (*application.App, *memory.Stack, *time.Time) {
	t.Helper()
	now := time.Now()
	clock := &now
	s := memory.NewStack(tpls...)
	app := application.New(application.Deps{
		Templates:   s.Templates,
		Instances:   s.Instances,
		Equips:      s.Equips,
		Idempotency: s.Idempotency,
		Publisher:   s.Bus,
		Ledger:      s.Ledger,
		Conditions:  &memory.ConditionChecker{Levels: s.Levels, Relations: s.Relations},
		Relations:   s.Relations,
		Banner:      s.Banner,
		NewID:       memory.NewIDGenerator("inst_").Next,
		Now:         func() time.Time { return *clock },
		Rand:        func(int64) int64 { return 0 },
	})
	s.App = app
	return app, s, clock
}

func forceExpire(t *testing.T, s *memory.Stack, instanceID string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	inst, err := s.Instances.Get(ctx, instanceID)
	if err != nil {
		t.Fatalf("get instance: %v", err)
	}
	expect := inst.Version
	inst.ExpireAt = &at
	inst.BumpVersion()
	if err := s.Instances.Update(ctx, inst, expect); err != nil {
		t.Fatalf("force expire: %v", err)
	}
}

func TestMountEquipRequiresLevel(t *testing.T) {
	ctx := context.Background()
	app, s, _ := newStackWithClock(t, catalog.Mounts())

	r, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "mount_dragon", Count: 1})
	if err != nil {
		t.Fatalf("grant: %v", err)
	}
	if err := app.Equip(ctx, "p1", r.InstanceID); err == nil {
		t.Fatalf("expected condition failure for low level")
	}

	s.Levels.SetLevel("p1", 30)
	if err := app.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip at level 30: %v", err)
	}

	snap, err := app.BuildProfile(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	mounts := snap.Slots[model.SlotMount]
	if len(mounts) != 1 || mounts[0].TemplateID != "mount_dragon" {
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
	app, s, _ := newStackWithClock(t, catalog.Mounts())
	s.Levels.SetLevel("p1", 99)

	dragon, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "mount_dragon", Count: 1})
	cloud, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "mount_cloud", Count: 1})
	if err := app.Equip(ctx, "p1", dragon.InstanceID); err != nil {
		t.Fatalf("equip dragon: %v", err)
	}
	if err := app.Equip(ctx, "p1", cloud.InstanceID); err != profile.ErrSlotFull {
		t.Fatalf("expected ErrSlotFull, got %v", err)
	}
	if err := app.Unequip(ctx, "p1", dragon.InstanceID); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	if err := app.Equip(ctx, "p1", cloud.InstanceID); err != nil {
		t.Fatalf("equip cloud after unequip: %v", err)
	}
}

func TestMountExpirableRemoved(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.Mounts())
	s.Levels.SetLevel("p1", 20)

	r, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "mount_ember_7d", Count: 1})
	if err := app.Equip(ctx, "p1", r.InstanceID); err != nil {
		t.Fatalf("equip: %v", err)
	}

	forceExpire(t, s, r.InstanceID, clock.Add(-time.Minute))
	n, err := app.RunExpiry(ctx, 10)
	if err != nil || n != 1 {
		t.Fatalf("expiry run: n=%d err=%v", n, err)
	}
	if _, err := s.Instances.Get(ctx, r.InstanceID); err == nil {
		t.Fatalf("expired mount should be removed")
	}
	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotMount]) != 0 {
		t.Fatalf("mount slot should be empty: %+v", snap.Slots[model.SlotMount])
	}
}
