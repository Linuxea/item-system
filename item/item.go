// Package item 定义道具体系的核心词汇：道具身份、能力接口、运行时实例。
//
// 设计要点：一个道具「有哪些能力」由它「实现了哪些接口」决定，编译期确定，无需任何配置。
// 引擎全程只认识本包的接口，永远不认识「座驾」「改名卡」这类具体类型。
package item

import (
	"context"
	"time"
)

// Item 所有道具的共同点：只有身份，没有行为。
// 行为一律通过下面的可选能力接口表达。
type Item interface {
	// ID 道具唯一标识，实例通过它关联回道具。
	ID() string
	// Name 展示名称。
	Name() string
}

// ============================================================
// 能力接口（全部可选）
//
// 引擎用类型断言询问：item.(Stackable) —— 实现了就有这个能力，没实现就没有。
// 新增一种能力 = 本文件加一个接口 + 引擎里加一处断言，其余道具不受影响。
// ============================================================

// Usable 可主动使用。
// Use 就是这个道具的全部业务逻辑；需要玩家输入参数的道具同时实现 Parameterized。
// ConsumePerUse 声明「一次使用消耗几个」，真正的扣减由引擎完成（要走乐观锁与幂等）。
type Usable interface {
	Use(ctx context.Context, owner string) error
	ConsumePerUse() int64
}

// Parameterized 使用前需要绑定玩家输入的道具（如改名卡的新昵称、飘屏的文案）。
//
// 注册表里存的是「原型」：只带依赖，参数字段为零值。
// 引擎在使用前调用 Bind，得到一个填好参数的副本，原型不被修改（并发安全）。
// 参数校验写在 Bind 里，失败时返回错误 —— 此刻尚未产生任何副作用，道具不会被扣。
type Parameterized interface {
	Bind(raw []byte) (Usable, error)
}

// Stackable 可堆叠：同种道具在背包里合并成一格，MaxStack 为单格数量上限。
// 未实现本接口的道具每次发放都是独立一格。
type Stackable interface {
	MaxStack() int64
}

// Equippable 可穿戴：Slot 目标槽位，Capacity 该槽位能同时穿戴几件。
type Equippable interface {
	Slot() Slot
	Capacity() int
}

// Conditional 穿戴前置条件：由道具自己判断，引擎只调用不代劳。
// 等级门槛、关系要求都走这里。
type Conditional interface {
	CanEquip(ctx context.Context, owner string) error
}

// Passive 被动属性：穿戴后生效的修饰符，由展示快照聚合。
type Passive interface {
	Modifiers() []Modifier
}

// Expirable 有时效：Duration 为发放后的有效期，OnExpire 为到期处置方式。
type Expirable interface {
	Duration() time.Duration
	OnExpire() ExpirePolicy
}

// Downgradable 到期降级的道具（OnExpire 返回 ExpireDowngrade 时必须同时实现）。
// DowngradeTo 返回目标道具 ID；目标道具自身若也有时效，降级后按目标的时效重新计时，
// 由此形成 vip3 → vip1 → vip0 的降级链。
type Downgradable interface {
	DowngradeTo() string
}

// Bindable 可绑定：BindOnGrant 为 true 时发放即绑定，绑定后不可交易。
type Bindable interface {
	BindOnGrant() bool
}

// Displayable 参与展示排序的道具：Priority 越大越靠前，Rarity 为稀有度。
// 未实现本接口的道具按 0 处理。
type Displayable interface {
	Priority() int
	Rarity() int
}

// ============================================================
// 值类型
// ============================================================

// Slot 穿戴槽位。同槽位的道具共享容量约束，并在展示快照中一起排序。
type Slot string

const (
	SlotAvatar     Slot = "avatar"      // 头像框
	SlotNamePlate  Slot = "nameplate"   // 铭牌
	SlotBadge      Slot = "badge"       // 勋章（容量可大于 1）
	SlotVIP        Slot = "vip"         // 会员身份
	SlotMount      Slot = "mount"       // 座驾
	SlotHeadwear   Slot = "headwear"    // 头饰
	SlotRelation   Slot = "relation"    // 关系卡
	SlotCPRing     Slot = "cp_ring"     // CP 戒指
	SlotChatBubble Slot = "chat_bubble" // 聊天气泡
)

// ExpirePolicy 到期处置策略。
type ExpirePolicy string

const (
	// ExpireRemove 到期删除实例（若在穿戴中，一并卸下）。
	ExpireRemove ExpirePolicy = "remove"
	// ExpireUnequip 到期仅卸下，实例保留在背包。
	ExpireUnequip ExpirePolicy = "unequip"
	// ExpireDowngrade 到期降级为另一款道具，需同时实现 Downgradable。
	ExpireDowngrade ExpirePolicy = "downgrade"
)

// Modifier 修饰符键值对，如 move_speed=80、vip_level=3。
// 由 Passive 能力携带，展示快照负责聚合。
type Modifier struct {
	Key   string
	Value any
}
