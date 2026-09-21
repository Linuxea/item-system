// lifecycle_test.go 全生命周期集成测试：堆叠合并、穿戴快照排序、
// 使用消耗、宝箱随机再发放、VIP 链式降级与事件留痕。
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

// newLifecycleStack 装配生命周期测试所需的全部道具定义。
func newLifecycleStack(t *testing.T) (*item.Inventory, *memory.Stack, *fixedRand, *time.Time) {
	t.Helper()
	inv, s, clock := newStack(t)
	rnd := &fixedRand{n: 0}
	var defs []item.Def
	defs = append(defs, catalog.AvatarFrames()...)
	defs = append(defs, catalog.Badges()...)
	defs = append(defs, catalog.VIPs()...)
	defs = append(defs, catalog.Potions(s.Ledger)...)
	defs = append(defs, catalog.Chests(rnd.next)...)
	s.Defs.Register(defs...)
	return inv, s, rnd, clock
}

func TestFullLifecycle(t *testing.T) {
	ctx := context.Background()
	inv, s, rnd, clock := newLifecycleStack(t)

	potion1, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "potion_hp", Count: 5})
	if err != nil {
		t.Fatalf("grant potion: %v", err)
	}
	potion2, err := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "potion_hp", Count: 3})
	if err != nil {
		t.Fatalf("grant potion: %v", err)
	}
	if potion1.InstanceID != potion2.InstanceID {
		t.Fatalf("potions should stack into one instance")
	}
	avatar, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "avatar_gold", Count: 1})
	abyss, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "badge_abyss", Count: 1})
	flame, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "badge_flame", Count: 1})
	vip, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "vip3_month", Count: 1})

	for _, r := range []struct {
		id  string
		err error
	}{
		{avatar.InstanceID, nil}, {abyss.InstanceID, nil}, {flame.InstanceID, nil}, {vip.InstanceID, nil},
	} {
		if err := inv.Equip(ctx, "p1", r.id); err != nil {
			t.Fatalf("equip %s: %v", r.id, err)
		}
	}
	if err := inv.Equip(ctx, "p1", potion1.InstanceID); !errors.Is(err, item.ErrNotEquippable) {
		t.Fatalf("expected ErrNotEquippable, got %v", err)
	}

	snap, err := inv.Build(ctx, "p1", "profile_home")
	if err != nil {
		t.Fatalf("profile: %v", err)
	}
	badges := snap.Slots[item.SlotBadge]
	if len(badges) != 2 || badges[0].DefID != "badge_abyss" || badges[1].DefID != "badge_flame" {
		t.Fatalf("badge order wrong: %+v", badges)
	}
	if len(snap.Slots[item.SlotAvatar]) != 1 || snap.Slots[item.SlotAvatar][0].DefID != "avatar_gold" {
		t.Fatalf("avatar slot wrong: %+v", snap.Slots[item.SlotAvatar])
	}
	if len(snap.Modifiers) != 2 {
		t.Fatalf("expected avatar+vip modifiers, got %+v", snap.Modifiers)
	}

	if _, err := inv.Use(ctx, item.UseRequest{Owner: "p1", InstanceID: potion1.InstanceID, Count: 2}); err != nil {
		t.Fatalf("use potion: %v", err)
	}
	if got := s.Ledger.Balance("p1", "hp"); got != 100 {
		t.Fatalf("expected hp=100, got %d", got)
	}
	inst, _ := s.Instances.Get(ctx, potion1.InstanceID)
	if inst.Count != 6 {
		t.Fatalf("expected 6 potions left, got %d", inst.Count)
	}

	chest, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "chest_basic", Count: 1})
	rnd.n = 0
	if _, err := inv.Use(ctx, item.UseRequest{Owner: "p1", InstanceID: chest.InstanceID}); err != nil {
		t.Fatalf("use chest: %v", err)
	}
	inst, _ = s.Instances.Get(ctx, potion1.InstanceID)
	if inst.Count != 9 {
		t.Fatalf("expected chest to grant 3 potions (6+3), got %d", inst.Count)
	}
	rnd.n = 71
	chest2, _ := inv.Grant(ctx, item.GrantRequest{Owner: "p1", DefID: "chest_basic", Count: 1})
	if _, err := inv.Use(ctx, item.UseRequest{Owner: "p1", InstanceID: chest2.InstanceID}); err != nil {
		t.Fatalf("use chest2: %v", err)
	}
	mine, _ := s.Instances.ListByOwner(ctx, "p1")
	flameCount := int64(0)
	for _, i := range mine {
		if i.DefID == "badge_flame" {
			flameCount += i.Count
		}
	}
	if flameCount != 2 {
		t.Fatalf("expected 2 flame badges, got %d", flameCount)
	}

	*clock = clock.Add(800 * time.Hour)
	n, err := inv.RunExpiry(ctx, 100)
	if err != nil || n != 1 {
		t.Fatalf("expiry run: n=%d err=%v", n, err)
	}
	vipInst, err := s.Instances.Get(ctx, vip.InstanceID)
	if err != nil {
		t.Fatalf("vip instance should survive downgrade: %v", err)
	}
	if vipInst.DefID != "vip1_week" || vipInst.Status != item.StatusEquipped || vipInst.ExpireAt == nil {
		t.Fatalf("downgrade wrong: %+v", vipInst)
	}
	if !vipInst.ExpireAt.After(*clock) {
		t.Fatalf("downgraded vip should inherit vip1 duration: %+v", vipInst.ExpireAt)
	}

	snap2, _ := inv.Build(ctx, "p1", "profile_home")
	if got := snap2.Slots[item.SlotVIP][0].DefID; got != "vip1_week" {
		t.Fatalf("expected vip1_week in slot after expiry, got %s", got)
	}

	var expiredSeen bool
	for _, e := range s.Bus.Recorded() {
		if ev, ok := e.(item.Expired); ok && ev.InstanceID == vip.InstanceID {
			expiredSeen = true
			if ev.Policy != item.PolicyDowngrade {
				t.Fatalf("wrong expiry policy: %s", ev.Policy)
			}
		}
	}
	if !expiredSeen {
		t.Fatalf("expected ItemExpired event")
	}

	if _, err := inv.Use(ctx, item.UseRequest{Owner: "p1", InstanceID: potion1.InstanceID, Count: 99}); !errors.Is(err, item.ErrInsufficient) {
		t.Fatalf("expected ErrInsufficient, got %v", err)
	}
}
