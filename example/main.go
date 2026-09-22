// Command example 端到端演示整套道具体系。
//
//	go run ./example
package main

import (
	"context"
	"fmt"
	"time"

	"github.com/Linuxea/item-system/engine"
	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/memory"
)

func main() {
	ctx := context.Background()

	// 一行装配出完整体系：引擎、关系服务、全部端口的内存实现。
	s := memory.NewStack(memory.StackOptions{
		Now:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Rand: func(int64) int64 { return 0 }, // 固定随机，便于演示
		Scenes: map[string]engine.SortPolicy{
			"chat": {engine.ByRarityDesc, engine.ByPriorityDesc},
		},
	})
	s.Levels.Set("u1", 30)
	s.Levels.Set("u2", 30)

	section("发放与堆叠")
	a := mustGrant(s, "u1", "rename_card", 3)
	b := mustGrant(s, "u1", "rename_card", 2)
	inst, _ := s.Instances.Get(ctx, a)
	fmt.Printf("  两次发放合并进同一实例 %s == %s，共 %d 张\n", a, b, inst.Count)

	section("使用改名卡 —— 参数是道具自己的字段")
	show(s.Engine.Use(ctx, engine.UseRequest{Owner: "u1", InstanceID: a}))
	fmt.Println("  ↑ 没传参数，零副作用")
	show(s.Engine.Use(ctx, engine.UseRequest{
		Owner: "u1", InstanceID: a, Params: []byte(`{"new_name":"林"}`),
	}))
	fmt.Println("  ↑ 昵称太短，校验写在改名卡自己的 Bind 里")
	show(s.Engine.Use(ctx, engine.UseRequest{
		Owner: "u1", InstanceID: a, Params: []byte(`{"new_name":"林夕"}`),
	}))
	fmt.Printf("  当前昵称：%s\n", s.Users.NameOf("u1"))

	section("能力由接口决定")
	mount := mustGrant(s, "u1", "mount_dragon", 1)
	show(s.Engine.Use(ctx, engine.UseRequest{Owner: "u1", InstanceID: mount}))
	fmt.Println("  ↑ 座驾没有 Use 方法")
	show2(s.Engine.Equip(ctx, "u1", a))
	fmt.Println("  ↑ 改名卡没有 Slot 方法")

	section("穿戴：等级门槛与槽位容量")
	s.Levels.Set("u1", 12)
	show2(s.Engine.Equip(ctx, "u1", mount))
	fmt.Println("  ↑ 游龙战车需 30 级，门槛由座驾自己判断")
	s.Levels.Set("u1", 30)
	show2(s.Engine.Equip(ctx, "u1", mount))
	cloud := mustGrant(s, "u1", "mount_cloud", 1)
	show2(s.Engine.Equip(ctx, "u1", cloud))
	fmt.Println("  ↑ mount 槽容量只有 1")

	section("勋章槽容量 3，可同时佩戴")
	for _, id := range []string{"badge_abyss", "badge_flame", "badge_rookie"} {
		show2(s.Engine.Equip(ctx, "u1", mustGrant(s, "u1", id, 1)))
	}

	section("展示快照（chat 场景按稀有度排序）")
	snap, _ := s.Engine.BuildProfile(ctx, "u1", "chat")
	for _, slot := range []item.Slot{item.SlotMount, item.SlotBadge} {
		fmt.Printf("  %-8s", slot)
		for _, e := range snap.Slots[slot] {
			fmt.Printf("[%s r%d] ", e.Name, e.Rarity)
		}
		fmt.Println()
	}
	fmt.Printf("  被动属性 %v\n", snap.Modifiers)

	section("CP 戒指需要先有关系")
	ring := mustGrant(s, "u1", "ring_eternal", 1)
	show2(s.Engine.Equip(ctx, "u1", ring))
	rel, _ := s.Relations.Bind(ctx, "cp", "u1", "u2", 0)
	fmt.Println("  建立 cp 关系 →")
	show2(s.Engine.Equip(ctx, "u1", ring))

	section("解除关系联动卸下")
	_ = s.Relations.Dissolve(ctx, rel.ID, "breakup")
	ri, _ := s.Instances.Get(ctx, ring)
	fmt.Printf("  戒指还在背包：%v，穿戴状态：%v\n", ri != nil, ri.Equipped)

	section("VIP 降级链：月卡 → 周卡 → 体验卡 → 删除")
	vip := mustGrant(s, "u1", "vip_month", 1)
	_ = s.Engine.Equip(ctx, "u1", vip)
	for _, step := range []struct {
		adv   time.Duration
		label string
	}{
		{31 * 24 * time.Hour, "月卡 30 天后"},
		{8 * 24 * time.Hour, "周卡 7 天后"},
		{4 * 24 * time.Hour, "体验卡 3 天后"},
	} {
		s.Clock.Advance(step.adv)
		_, _ = s.Engine.RunExpiry(ctx)
		fmt.Printf("  %-14s当前持有：%s\n", step.label, vipOf(s, "u1"))
	}

	section("宝箱：用了之后再发别的道具")
	chest := mustGrant(s, "u1", "chest_starter", 3)
	res, _ := s.Engine.Use(ctx, engine.UseRequest{Owner: "u1", InstanceID: chest, Count: 3})
	fmt.Printf("  连开 %d 次，获得金币 %d\n", res.Consumed, goldOf(s, "u1"))

	section("领域事件")
	counts := map[string]int{}
	for _, e := range s.Bus.Events() {
		counts[string(e.Kind)]++
	}
	for _, k := range []string{"item.granted", "item.consumed", "item.equipped",
		"item.unequipped", "item.expired", "item.downgraded",
		"relation.bound", "relation.dissolved"} {
		if n := counts[k]; n > 0 {
			fmt.Printf("  %-22s %d\n", k, n)
		}
	}
}

func section(title string) { fmt.Printf("\n── %s ──\n", title) }

func mustGrant(s *memory.Stack, owner, itemID string, n int64) string {
	res, err := s.Engine.Grant(context.Background(), engine.GrantRequest{
		Owner: owner, ItemID: itemID, Count: n, Source: "demo",
	})
	if err != nil {
		panic(err)
	}
	return res.InstanceID
}

func show(res *engine.UseResult, err error) {
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		return
	}
	fmt.Printf("  ✓ 消耗 %d，剩余 %d\n", res.Consumed, res.Remaining)
}

func show2(err error) {
	if err != nil {
		fmt.Printf("  ✗ %v\n", err)
		return
	}
	fmt.Println("  ✓ ok")
}

func vipOf(s *memory.Stack, owner string) string {
	owned, _ := s.Instances.ListByOwner(context.Background(), owner)
	for _, inst := range owned {
		if len(inst.ItemID) >= 3 && inst.ItemID[:3] == "vip" {
			it, _ := s.Registry.Lookup(inst.ItemID)
			return it.Name()
		}
	}
	return "（已清空）"
}

func goldOf(s *memory.Stack, owner string) int64 {
	owned, _ := s.Instances.ListByOwner(context.Background(), owner)
	var n int64
	for _, inst := range owned {
		if inst.ItemID == "pack_gold_100" {
			n += inst.Count
		}
	}
	return n
}
