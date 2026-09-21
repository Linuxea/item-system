package application_test

import (
	"context"
	"testing"

	"github.com/Linuxea/item-system/catalog"
	"github.com/Linuxea/item-system/domain/grant"
	"github.com/Linuxea/item-system/domain/model"
	"github.com/Linuxea/item-system/domain/profile"
)

func grantAndEquip(t *testing.T, app interface {
	Grant(ctx context.Context, req grant.Request) (*model.GrantResult, error)
	Equip(ctx context.Context, owner, instanceID string) error
}, owner, templateID string) string {
	t.Helper()
	ctx := context.Background()
	r, err := app.Grant(ctx, grant.Request{Owner: owner, TemplateID: templateID, Count: 1})
	if err != nil {
		t.Fatalf("grant %s: %v", templateID, err)
	}
	if err := app.Equip(ctx, owner, r.InstanceID); err != nil {
		t.Fatalf("equip %s: %v", templateID, err)
	}
	return r.InstanceID
}

func TestBadgeSlotCapacityAndOrder(t *testing.T) {
	ctx := context.Background()
	app, _, _ := newStackWithClock(t, catalog.Badges())

	grantAndEquip(t, app, "p1", "badge_rookie")
	grantAndEquip(t, app, "p1", "badge_flame")
	grantAndEquip(t, app, "p1", "badge_abyss")

	abyss, err := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "badge_abyss", Count: 1})
	if err != nil {
		t.Fatalf("grant duplicate: %v", err)
	}
	if err := app.Equip(ctx, "p1", abyss.InstanceID); err == nil {
		t.Fatalf("expected equip failure: instance already equipped")
	}

	rookie2, _ := app.Grant(ctx, grant.Request{Owner: "p1", TemplateID: "badge_rookie", Count: 1})
	if err := app.Equip(ctx, "p1", rookie2.InstanceID); err != profile.ErrSlotFull {
		t.Fatalf("expected ErrSlotFull on 4th badge, got %v", err)
	}

	snap, err := app.BuildProfile(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	badges := snap.Slots[model.SlotBadge]
	if len(badges) != 3 {
		t.Fatalf("expected 3 badges, got %d", len(badges))
	}
	want := []string{"badge_abyss", "badge_flame", "badge_rookie"}
	for i, w := range want {
		if badges[i].TemplateID != w {
			t.Fatalf("pos %d: want %s got %s", i, w, badges[i].TemplateID)
		}
	}
}

func TestBadgeUnequipFreesHole(t *testing.T) {
	ctx := context.Background()
	app, _, _ := newStackWithClock(t, catalog.Badges())

	id := grantAndEquip(t, app, "p1", "badge_rookie")
	grantAndEquip(t, app, "p1", "badge_flame")

	if err := app.Unequip(ctx, "p1", id); err != nil {
		t.Fatalf("unequip: %v", err)
	}
	grantAndEquip(t, app, "p1", "badge_abyss")

	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if got := len(snap.Slots[model.SlotBadge]); got != 2 {
		t.Fatalf("expected 2 badges after swap, got %d", got)
	}
}
