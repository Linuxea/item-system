package application_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/catalog"
	"github.com/Linuxea/item-system/domain/model"
	"github.com/Linuxea/item-system/domain/profile"
)

func TestChatBubbleEquipAndScenePolicy(t *testing.T) {
	ctx := context.Background()
	app, _, _ := newStackWithClock(t, append(catalog.ChatBubbles(), catalog.Badges()...))
	app.Sorts.Register("chat", profile.TopN(1, profile.DefaultSort))

	id := grantAndEquip(t, app, "p1", "bubble_star")
	grantAndEquip(t, app, "p1", "badge_abyss")
	grantAndEquip(t, app, "p1", "badge_rookie")

	home, _ := app.BuildProfile(ctx, "p1", "profile_home")
	bubbles := home.Slots[model.SlotChatBubble]
	if len(bubbles) != 1 || bubbles[0].TemplateID != "bubble_star" {
		t.Fatalf("bubble slot wrong: %+v", bubbles)
	}
	if got := len(home.Slots[model.SlotBadge]); got != 2 {
		t.Fatalf("home scene should show both badges, got %d", got)
	}

	chat, _ := app.BuildProfile(ctx, "p1", "chat")
	if got := len(chat.Slots[model.SlotBadge]); got != 1 {
		t.Fatalf("chat scene should show top1 badge, got %d", got)
	}
	if chat.Slots[model.SlotBadge][0].TemplateID != "badge_abyss" {
		t.Fatalf("chat scene badge wrong: %+v", chat.Slots[model.SlotBadge])
	}
	if got := len(chat.Slots[model.SlotChatBubble]); got != 1 {
		t.Fatalf("chat scene bubble wrong: %+v", chat.Slots[model.SlotChatBubble])
	}

	if err := app.Unequip(ctx, "p1", id); err != nil {
		t.Fatalf("unequip: %v", err)
	}
}

func TestChatBubbleExpirableReturnsToNormal(t *testing.T) {
	ctx := context.Background()
	app, s, clock := newStackWithClock(t, catalog.ChatBubbles())

	id := grantAndEquip(t, app, "p1", "bubble_aurora")

	forceExpire(t, s, id, clock.Add(-time.Second))
	if n, err := app.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry: n=%d err=%v", n, err)
	}
	inst, err := s.Instances.Get(ctx, id)
	if err != nil || inst.Status != model.StatusNormal {
		t.Fatalf("expired bubble should unequip and persist: %+v err=%v", inst, err)
	}
	snap, _ := app.BuildProfile(ctx, "p1", "profile_home")
	if len(snap.Slots[model.SlotChatBubble]) != 0 {
		t.Fatalf("bubble slot should be empty")
	}
}
