package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/catalog"
	"github.com/Linuxea/item-system/domain/event"
	"github.com/Linuxea/item-system/domain/grant"
	"github.com/Linuxea/item-system/domain/model"
	"github.com/Linuxea/item-system/domain/profile"
)

func TestNameplatePermanentAndExpirable(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.Nameplates())

	festival := grantAndEquip(t, app, "p1", "np_festival_72h")
	perm, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "np_ember", Count: 1})
	if err != nil {
		t.Fatalf("grant perm plate: %v", err)
	}
	if err := app.Equip(ctx, "p1", perm.InstanceID); err != profile.ErrSlotFull {
		t.Fatalf("expected ErrSlotFull for exclusive nameplate, got %v", err)
	}

	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotNamePlate]) != 1 || snap.Slots[model.SlotNamePlate][0].TemplateID != "np_festival_72h" {
		t.Fatalf("nameplate slot wrong: %+v", snap.Slots[model.SlotNamePlate])
	}

	if err := app.Unequip(ctx, "p1", festival); err != nil {
		t.Fatalf("unequip festival: %v", err)
	}
	forceExpire(t, s, festival, clock.Add(-time.Second))
	if n, err := app.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry: n=%d err=%v", n, err)
	}

	if _, err := s.Instances.Get(ctx, festival); err == nil {
		t.Fatalf("expired festival plate should be removed")
	}

	var evicted bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(event.Expired); ok && ev.InstanceID == festival {
			evicted = true
			if ev.Policy != "remove" {
				t.Fatalf("wrong policy: %s", ev.Policy)
			}
		}
	}
	if !evicted {
		t.Fatalf("expected expiry event")
	}

	if err := app.Equip(ctx, "p1", perm.InstanceID); err != nil {
		t.Fatalf("equip permanent plate: %v", err)
	}
	snap, _ = app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotNamePlate]) != 1 || snap.Slots[model.SlotNamePlate][0].TemplateID != "np_ember" {
		t.Fatalf("permanent plate should be equippable after removal: %+v", snap.Slots[model.SlotNamePlate])
	}
}
