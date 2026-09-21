// chat_bubble_test.go 聊天气泡：穿戴、场景排序策略（chat 场景只展示 TopN 勋章）、
// 限时款到期卸下保留。
package item_test

import (
	"context"
	"testing"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func bubbleStack(t *testing.T, extra ...item.Def) (*item.Inventory, *memory.Stack, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	s.Defs.Register(catalog.ChatBubbles()...)
	s.Defs.Register(extra...)
	return inv, s, clock
}

func TestChatBubbleEquipAndScenePolicy(t *testing.T) {
	ctx := context.Background()
	inv, _, _ := bubbleStack(t, catalog.Badges()...)
	inv.Sorts.Register("chat", item.TopN(1, item.DefaultSort))

	id := grantAndEquip(t, inv, "p1", "bubble_star")
	grantAndEquip(t, inv, "p1", "badge_abyss")
	grantAndEquip(t, inv, "p1", "badge_rookie")

	home, _ := inv.Build(ctx, "p1", "profile_home")
	bubbles := home.Slots[item.SlotChatBubble]
	if len(bubbles) != 1 || bubbles[0].DefID != "bubble_star" {
		t.Fatalf("bubble slot wrong: %+v", bubbles)
	}
	if got := len(home.Slots[item.SlotBadge]); got != 2 {
		t.Fatalf("home scene should show both badges, got %d", got)
	}

	chat, _ := inv.Build(ctx, "p1", "chat")
	if got := len(chat.Slots[item.SlotBadge]); got != 1 {
		t.Fatalf("chat scene should show top1 badge, got %d", got)
	}
	if chat.Slots[item.SlotBadge][0].DefID != "badge_abyss" {
		t.Fatalf("chat scene badge wrong: %+v", chat.Slots[item.SlotBadge])
	}
	if got := len(chat.Slots[item.SlotChatBubble]); got != 1 {
		t.Fatalf("chat scene bubble wrong: %+v", chat.Slots[item.SlotChatBubble])
	}

	if err := inv.Unequip(ctx, "p1", id); err != nil {
		t.Fatalf("unequip: %v", err)
	}
}

func TestChatBubbleExpirableReturnsToNormal(t *testing.T) {
	ctx := context.Background()
	inv, s, clock := bubbleStack(t)

	id := grantAndEquip(t, inv, "p1", "bubble_aurora")

	forceExpire(t, s, id, clock.Add(-time.Second))
	if n, err := inv.RunExpiry(ctx, 10); err != nil || n != 1 {
		t.Fatalf("expiry: n=%d err=%v", n, err)
	}
	inst, err := s.Instances.Get(ctx, id)
	if err != nil || inst.Status != item.StatusNormal {
		t.Fatalf("expired bubble should unequip and persist: %+v err=%v", inst, err)
	}
	snap, _ := inv.Build(ctx, "p1", "profile_home")
	if len(snap.Slots[item.SlotChatBubble]) != 0 {
		t.Fatalf("bubble slot should be empty")
	}
}
