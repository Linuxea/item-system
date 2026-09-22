package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Linuxea/item-system/engine"
	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/memory"
)

// TestEquipLevelGate 座驾的等级门槛由座驾自己判断。
func TestEquipLevelGate(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	s.Levels.Set("u1", 12)
	dragon := grant(t, s, "u1", "mount_dragon", 1) // 需 30 级

	err := s.Engine.Equip(ctx, "u1", dragon)
	if !errors.Is(err, item.ErrConditionNotMet) {
		t.Fatalf("应报未满足条件，实际: %v", err)
	}

	s.Levels.Set("u1", 30)
	if err := s.Engine.Equip(ctx, "u1", dragon); err != nil {
		t.Fatalf("升级后应可穿戴: %v", err)
	}
}

// TestEquipSlotCapacity 座驾槽容量 1，勋章槽容量 3。
func TestEquipSlotCapacity(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()

	cloud := grant(t, s, "u1", "mount_cloud", 1)
	dragon := grant(t, s, "u1", "mount_dragon", 1)
	if err := s.Engine.Equip(ctx, "u1", cloud); err != nil {
		t.Fatalf("第一辆应穿上: %v", err)
	}
	if err := s.Engine.Equip(ctx, "u1", dragon); !errors.Is(err, item.ErrSlotFull) {
		t.Fatalf("第二辆应报槽位已满，实际: %v", err)
	}

	// 勋章槽能戴三枚。
	for _, id := range []string{"badge_abyss", "badge_flame", "badge_rookie"} {
		inst := grant(t, s, "u1", id, 1)
		if err := s.Engine.Equip(ctx, "u1", inst); err != nil {
			t.Fatalf("勋章 %s 应可佩戴: %v", id, err)
		}
	}
	extra := grant(t, s, "u1", "badge_abyss", 1)
	if err := s.Engine.Equip(ctx, "u1", extra); !errors.Is(err, item.ErrSlotFull) {
		t.Fatalf("第四枚勋章应报槽位已满，实际: %v", err)
	}
}

// TestEquipNotEquippable 改名卡没有 Slot 方法，编译期就不是可穿戴道具。
func TestEquipNotEquippable(t *testing.T) {
	s := newStack(t)
	id := grant(t, s, "u1", "rename_card", 1)
	if err := s.Engine.Equip(context.Background(), "u1", id); !errors.Is(err, item.ErrNotEquippable) {
		t.Fatalf("应报不可穿戴，实际: %v", err)
	}
}

// TestExpireRemove 限时座驾到期即删。
func TestExpireRemove(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	id := grant(t, s, "u1", "mount_ember_7d", 1)
	if err := s.Engine.Equip(ctx, "u1", id); err != nil {
		t.Fatalf("应可穿戴: %v", err)
	}

	s.Clock.Advance(8 * 24 * time.Hour)
	res, err := s.Engine.RunExpiry(ctx)
	if err != nil {
		t.Fatalf("过期扫描失败: %v", err)
	}
	if len(res.Removed) != 1 {
		t.Fatalf("应删除 1 个实例，实际 %+v", res)
	}
	if _, err := s.Instances.Get(ctx, id); !errors.Is(err, item.ErrNoSuchInstance) {
		t.Fatal("实例应已被删除")
	}
}

// TestExpireUnequip 限时头饰到期仅卸下，实例保留。
func TestExpireUnequip(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	id := grant(t, s, "u1", "cap_sprint_3d", 1)
	if err := s.Engine.Equip(ctx, "u1", id); err != nil {
		t.Fatalf("应可穿戴: %v", err)
	}

	s.Clock.Advance(4 * 24 * time.Hour)
	res, err := s.Engine.RunExpiry(ctx)
	if err != nil {
		t.Fatalf("过期扫描失败: %v", err)
	}
	if len(res.Unequipped) != 1 {
		t.Fatalf("应卸下 1 个实例，实际 %+v", res)
	}
	inst, err := s.Instances.Get(ctx, id)
	if err != nil {
		t.Fatalf("实例应保留: %v", err)
	}
	if inst.Equipped {
		t.Fatal("实例应已卸下")
	}

	// 再扫一次不应重复处置。
	again, _ := s.Engine.RunExpiry(ctx)
	if len(again.Unequipped) != 0 {
		t.Fatal("已处置过的实例不该被反复捡起")
	}
}

// TestVIPDowngradeChain 月卡 -> 周卡 -> 体验卡 -> 删除。
// 每一档只知道自己的下一档，整条链没有任何一处集中保存。
func TestVIPDowngradeChain(t *testing.T) {
	s := newStack(t)
	ctx := context.Background()
	id := grant(t, s, "u1", "vip_month", 1)
	if err := s.Engine.Equip(ctx, "u1", id); err != nil {
		t.Fatalf("应可穿戴: %v", err)
	}

	currentItemOf := func(owner string) string {
		owned, _ := s.Instances.ListByOwner(ctx, owner)
		for _, inst := range owned {
			return inst.ItemID
		}
		return ""
	}

	// 第一环：月卡 30 天到期 -> 周卡
	s.Clock.Advance(31 * 24 * time.Hour)
	res, err := s.Engine.RunExpiry(ctx)
	if err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if len(res.Downgraded) != 1 || res.Downgraded[0].To != "vip_week" {
		t.Fatalf("应降级为周卡，实际 %+v", res.Downgraded)
	}
	if got := currentItemOf("u1"); got != "vip_week" {
		t.Fatalf("当前应为 vip_week，实际 %s", got)
	}

	// 第二环：周卡 7 天到期 -> 体验卡
	s.Clock.Advance(8 * 24 * time.Hour)
	if _, err := s.Engine.RunExpiry(ctx); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	if got := currentItemOf("u1"); got != "vip_trial" {
		t.Fatalf("当前应为 vip_trial，实际 %s", got)
	}

	// 第三环：体验卡 3 天到期 -> 删除
	s.Clock.Advance(4 * 24 * time.Hour)
	if _, err := s.Engine.RunExpiry(ctx); err != nil {
		t.Fatalf("扫描失败: %v", err)
	}
	owned, _ := s.Instances.ListByOwner(ctx, "u1")
	if len(owned) != 0 {
		t.Fatalf("体验卡到期后应清空，实际还剩 %d 个", len(owned))
	}
}

// TestProfileSnapshot 展示快照按槽位聚合，并合并全部被动属性。
func TestProfileSnapshot(t *testing.T) {
	s := memory.NewStack(memory.StackOptions{
		Now: epoch,
		Scenes: map[string]engine.SortPolicy{
			// 聊天场景：先看稀有度，再看优先级。
			"chat": {engine.ByRarityDesc, engine.ByPriorityDesc},
		},
	})
	s.Levels.Set("u1", 50)
	ctx := context.Background()

	mount := grant(t, s, "u1", "mount_dragon", 1)
	bubble := grant(t, s, "u1", "bubble_starry", 1)
	b1 := grant(t, s, "u1", "badge_rookie", 1) // rarity 1
	b2 := grant(t, s, "u1", "badge_abyss", 1)  // rarity 5

	for _, id := range []string{mount, bubble, b1, b2} {
		if err := s.Engine.Equip(ctx, "u1", id); err != nil {
			t.Fatalf("穿戴失败: %v", err)
		}
	}

	snap, err := s.Engine.BuildProfile(ctx, "u1", "chat")
	if err != nil {
		t.Fatalf("构建快照失败: %v", err)
	}
	if got := snap.Modifiers["move_speed"]; got != int64(150) {
		t.Fatalf("移速应为 150，实际 %v", got)
	}
	if got := snap.Modifiers["bubble_style"]; got != "starry" {
		t.Fatalf("气泡样式不对: %v", got)
	}
	badges := snap.Slots[item.SlotBadge]
	if len(badges) != 2 {
		t.Fatalf("勋章槽应有 2 枚，实际 %d", len(badges))
	}
	if badges[0].ItemID != "badge_abyss" {
		t.Fatalf("chat 场景按稀有度排序，深渊勋章应在前，实际 %s", badges[0].ItemID)
	}
}
