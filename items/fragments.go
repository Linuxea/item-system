// Package items 是全部具体道具的实现。
//
// 每个道具是一个 struct，它「有什么能力」由它「实现了哪些 item 包的接口」决定。
// 本文件提供几个可嵌入的能力片段：需要某项能力时把片段嵌进去，就获得了对应的方法，
// 不必每个道具重写一遍。这就是 Go 的组合 —— 编译期完成，无需任何配置。
package items

import (
	"context"
	"fmt"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/port"
)

// identity 身份片段：几乎所有道具都需要。
type identity struct {
	id   string
	name string
}

// ID 道具唯一标识。
func (i identity) ID() string { return i.id }

// Name 展示名称。
func (i identity) Name() string { return i.name }

// display 展示排序片段：嵌入后即实现 item.Displayable。
type display struct {
	priority int
	rarity   int
}

// Priority 展示优先级，越大越靠前。
func (d display) Priority() int { return d.priority }

// Rarity 稀有度。
func (d display) Rarity() int { return d.rarity }

// wearable 穿戴片段：嵌入后即实现 item.Equippable。
type wearable struct {
	slot     item.Slot
	capacity int
}

// Slot 目标槽位。
func (w wearable) Slot() item.Slot { return w.slot }

// Capacity 该槽位可同时穿戴的件数。
func (w wearable) Capacity() int { return w.capacity }

// timed 时效片段：嵌入后即实现 item.Expirable。
// duration 为 0 时引擎不会给实例设置到期时间，等同于永久 —— 因此
// 「有的款限时、有的款永久」不需要两个类型，只需构造时传不同的 duration。
type timed struct {
	duration time.Duration
	onExpire item.ExpirePolicy
}

// Duration 有效期；0 表示永久。
func (t timed) Duration() time.Duration { return t.duration }

// OnExpire 到期处置策略。
func (t timed) OnExpire() item.ExpirePolicy {
	if t.onExpire == "" {
		return item.ExpireRemove
	}
	return t.onExpire
}

// levelGate 等级门槛片段：嵌入后即实现 item.Conditional。
// minLevel 为 0 或未注入等级来源时放行。
type levelGate struct {
	minLevel int64
	levels   port.LevelSource
}

// CanEquip 校验玩家等级。
func (g levelGate) CanEquip(ctx context.Context, owner string) error {
	if g.minLevel <= 0 || g.levels == nil {
		return nil
	}
	lv, err := g.levels.LevelOf(ctx, owner)
	if err != nil {
		return err
	}
	if lv < g.minLevel {
		return fmt.Errorf("%w: 需要 %d 级（当前 %d 级）", item.ErrConditionNotMet, g.minLevel, lv)
	}
	return nil
}

// relationGate 关系门槛片段：嵌入后即实现 item.Conditional。
// 关系卡、CP 戒指要求玩家先与他人建立对应关系才能穿戴。
type relationGate struct {
	requires string
	checker  port.RelationChecker
}

// CanEquip 校验玩家是否已建立所需关系。
func (g relationGate) CanEquip(ctx context.Context, owner string) error {
	if g.requires == "" || g.checker == nil {
		return nil
	}
	ok, err := g.checker.HasActive(ctx, owner, g.requires)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%w: 需要先建立「%s」关系", item.ErrConditionNotMet, g.requires)
	}
	return nil
}

// stack 堆叠片段：嵌入后即实现 item.Stackable。
type stack struct{ max int64 }

// MaxStack 单格数量上限。
func (s stack) MaxStack() int64 { return s.max }
