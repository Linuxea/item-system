// Package catalog 提供各类道具的具体类型示例：每种道具一个显式 Go 类型，
// 行为写在类型的方法里（飘屏卡.Use 直接调广播端口、座驾.CanEquip 直接查等级源），
// 展示标签内嵌 item.Common。这里是"新道具 = 新类型 + 实现想要的能力接口"的落地示范。
//
// 道具需要的外部服务（等级源、广播端口、账本、改名服务、随机源）作为
// 类型字段由构造方注入；同款不同配置（云朵座驾 vs 龙车）就是不同的构造值。
package catalog

import (
	"context"
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/relation"
)

// LevelSource 等级查询端口：座驾等有等级前置的道具持有它。
type LevelSource interface {
	// Level 返回玩家当前等级。
	Level(ctx context.Context, owner string) int64
}

// BannerBroadcaster 飘屏广播端口：飘屏卡持有它。
type BannerBroadcaster interface {
	// Broadcast 向全服广播一条飘屏。
	Broadcast(ctx context.Context, owner, text string, duration time.Duration) error
}

// RenameService 改名服务端口：改名卡持有它。
type RenameService interface {
	// Rename 把玩家昵称改为 newName。
	Rename(ctx context.Context, owner, newName string) error
}

// Ledger 货币账本端口：药水等数值型道具持有它。
type Ledger interface {
	// Add 给玩家某币种入账。
	Add(ctx context.Context, owner, currency string, amount int64) error
}

// ---------------------------------------------------------------------------
// 座驾：可穿戴（座驾槽独占）+ 等级前置 + 移速被动；限时款到期即删。

// Mount 座驾道具类型。
type Mount struct {
	item.Common
	// Speed 移速加成（被动修饰）。
	Speed int
	// MinLevel 穿戴所需最低等级。
	MinLevel int64
	// Levels 等级查询端口；nil 时视为无等级限制。
	Levels LevelSource
	// Duration 有效期，0 表示永久（限时款才设置）。
	Duration time.Duration
	// Policy 过期策略，空视为 remove。
	Policy string
}

var _ interface {
	item.Def
	item.Equippable
	item.EquipGated
	item.Passive
	item.Expirable
} = (*Mount)(nil)

// EquipSlot 返回座驾槽。
func (m *Mount) EquipSlot() item.Slot { return item.SlotMount }

// Capacity 座驾槽独占。
func (m *Mount) Capacity() int { return 1 }

// CanEquip 检查玩家等级是否达标；未注入等级源时视为无限制。
func (m *Mount) CanEquip(ctx context.Context, inv *item.Inventory, owner string) (bool, error) {
	if m.MinLevel <= 0 || m.Levels == nil {
		return true, nil
	}
	return m.Levels.Level(ctx, owner) >= m.MinLevel, nil
}

// Modifiers 返回移速被动。
func (m *Mount) Modifiers() []item.Modifier {
	if m.Speed == 0 {
		return nil
	}
	return []item.Modifier{{Key: "move_speed", Value: int64(m.Speed)}}
}

// Lifetime 限时款的有效期。
func (m *Mount) Lifetime() time.Duration { return m.Duration }

// ExpirePolicy 过期策略，默认删除。
func (m *Mount) ExpirePolicy() string {
	if m.Policy == "" {
		return item.PolicyRemove
	}
	return m.Policy
}

// DowngradeTo 座驾不支持降级。
func (m *Mount) DowngradeTo() string { return "" }

// Mounts 座驾示例集：云朵（1级）、龙车（30级）、限时余烬竞速（10级/7天）。
func Mounts(levels LevelSource) []item.Def {
	return []item.Def{
		&Mount{
			Common:   item.Common{ID: "mount_cloud", Category: item.CategoryMount, Name: "Cloud Steed", Priority: 65, Rarity: 3},
			Speed:    80,
			MinLevel: 1,
			Levels:   levels,
		},
		&Mount{
			Common:   item.Common{ID: "mount_dragon", Category: item.CategoryMount, Name: "Dragon Chariot", Priority: 85, Rarity: 5},
			Speed:    150,
			MinLevel: 30,
			Levels:   levels,
		},
		&Mount{
			Common:   item.Common{ID: "mount_ember_7d", Category: item.CategoryMount, Name: "Ember Racer (7d)", Priority: 70, Rarity: 4},
			Speed:    120,
			MinLevel: 10,
			Levels:   levels,
			Duration: 7 * 24 * time.Hour,
			Policy:   item.PolicyRemove,
		},
	}
}

// ---------------------------------------------------------------------------
// 勋章：多容量勋章槽并穿，无前置。

// Badge 勋章道具类型。
type Badge struct {
	item.Common
	// Slots 勋章槽可并穿数量（缺省 3）。
	Slots int
}

var _ interface {
	item.Def
	item.Equippable
} = (*Badge)(nil)

// EquipSlot 返回勋章槽。
func (b *Badge) EquipSlot() item.Slot { return item.SlotBadge }

// Capacity 返回并穿容量，默认 3。
func (b *Badge) Capacity() int {
	if b.Slots <= 0 {
		return 3
	}
	return b.Slots
}

// Badges 勋章示例集：深渊（高稀有）、烈焰（中）、新星（低）。
func Badges() []item.Def {
	return []item.Def{
		&Badge{Common: item.Common{ID: "badge_abyss", Category: item.CategoryBadge, Name: "Abyss Conqueror", Priority: 70, Rarity: 5}},
		&Badge{Common: item.Common{ID: "badge_flame", Category: item.CategoryBadge, Name: "Flame Veteran", Priority: 70, Rarity: 3}},
		&Badge{Common: item.Common{ID: "badge_rookie", Category: item.CategoryBadge, Name: "Rookie Star", Priority: 40, Rarity: 1}},
	}
}

// ---------------------------------------------------------------------------
// 头饰：头饰槽独占；限时款到期仅卸下保留。

// Headwear 头饰道具类型。
type Headwear struct {
	item.Common
	// Glow 头部光效（被动修饰），空则无被动。
	Glow string
	// Duration 有效期，0 表示永久。
	Duration time.Duration
	// Policy 过期策略，空视为 unequip。
	Policy string
}

var _ interface {
	item.Def
	item.Equippable
	item.Passive
	item.Expirable
} = (*Headwear)(nil)

// EquipSlot 返回头饰槽。
func (h *Headwear) EquipSlot() item.Slot { return item.SlotHeadwear }

// Capacity 头饰槽独占。
func (h *Headwear) Capacity() int { return 1 }

// Modifiers 返回头部光效被动。
func (h *Headwear) Modifiers() []item.Modifier {
	if h.Glow == "" {
		return nil
	}
	return []item.Modifier{{Key: "head_glow", Value: h.Glow}}
}

// Lifetime 限时款的有效期。
func (h *Headwear) Lifetime() time.Duration { return h.Duration }

// ExpirePolicy 过期策略，默认仅卸下保留。
func (h *Headwear) ExpirePolicy() string {
	if h.Policy == "" {
		return item.PolicyUnequip
	}
	return h.Policy
}

// DowngradeTo 头饰不支持降级。
func (h *Headwear) DowngradeTo() string { return "" }

// Headwears 头饰示例集：极光冠冕（永久+光效）、冲刺鸭舌帽（3天，到期卸下）。
func Headwears() []item.Def {
	return []item.Def{
		&Headwear{
			Common: item.Common{ID: "crown_aurora", Category: item.CategoryHeadwear, Name: "Aurora Crown", Priority: 88, Rarity: 5},
			Glow:   "aurora",
		},
		&Headwear{
			Common:   item.Common{ID: "cap_sprint_3d", Category: item.CategoryHeadwear, Name: "Sprint Cap (3d)", Priority: 55, Rarity: 2},
			Duration: 72 * time.Hour,
			Policy:   item.PolicyUnequip,
		},
	}
}

// ---------------------------------------------------------------------------
// VIP：VIP 槽独占，到期链式降级（月卡→周卡→vip0），等级进被动修饰。

// VIP 会员道具类型。
type VIP struct {
	item.Common
	// Level VIP 等级（被动修饰 vip_level）。
	Level int
	// Duration 有效期，0 表示永久（如 vip0）。
	Duration time.Duration
	// Downgrade 到期降级目标的道具标识；空则到期按 RemovePolicy 处理。
	Downgrade string
	// RemovePolicy 无降级目标时的过期策略，空视为 remove。
	RemovePolicy string
}

var _ interface {
	item.Def
	item.Equippable
	item.Passive
	item.Expirable
} = (*VIP)(nil)

// EquipSlot 返回 VIP 槽。
func (v *VIP) EquipSlot() item.Slot { return item.SlotVIP }

// Capacity VIP 槽独占。
func (v *VIP) Capacity() int { return 1 }

// Modifiers 返回 VIP 等级被动。
func (v *VIP) Modifiers() []item.Modifier {
	return []item.Modifier{{Key: "vip_level", Value: v.Level}}
}

// Lifetime 返回有效期。
func (v *VIP) Lifetime() time.Duration { return v.Duration }

// ExpirePolicy 声明降级目标时为 downgrade，否则按 RemovePolicy（默认 remove）。
func (v *VIP) ExpirePolicy() string {
	if v.Downgrade != "" {
		return item.PolicyDowngrade
	}
	if v.RemovePolicy == "" {
		return item.PolicyRemove
	}
	return v.RemovePolicy
}

// DowngradeTo 返回降级目标标识。
func (v *VIP) DowngradeTo() string { return v.Downgrade }

// VIPs VIP 示例集：月卡（降级周卡）、周卡（降级 vip0）、永久 vip0、试用卡（到期删除）。
func VIPs() []item.Def {
	return []item.Def{
		&VIP{
			Common:    item.Common{ID: "vip3_month", Category: item.CategoryVIP, Name: "VIP3 Monthly", Priority: 60, Rarity: 4},
			Level:     3,
			Duration:  30 * 24 * time.Hour,
			Downgrade: "vip1_week",
		},
		&VIP{
			Common:    item.Common{ID: "vip1_week", Category: item.CategoryVIP, Name: "VIP1 Weekly", Priority: 30, Rarity: 2},
			Level:     1,
			Duration:  7 * 24 * time.Hour,
			Downgrade: "vip0",
		},
		&VIP{
			Common: item.Common{ID: "vip0", Category: item.CategoryVIP, Name: "VIP0 Basic", Priority: 10, Rarity: 1},
			Level:  0,
		},
		&VIP{
			Common:       item.Common{ID: "vip_trial_1d", Category: item.CategoryVIP, Name: "VIP Trial (1d)", Priority: 30, Rarity: 1},
			Level:        1,
			Duration:     24 * time.Hour,
			RemovePolicy: item.PolicyRemove,
		},
	}
}

// ---------------------------------------------------------------------------
// 关系卡：穿戴前置为对应类型的生效关系；关系解除时由通用层联动卸下。

// RelationCard 关系卡道具类型。
type RelationCard struct {
	item.Common
	// Requires 穿戴所依赖的关系类型。
	Requires relation.Type
	// Cap 关系卡槽容量（如闺蜜卡可并穿 2 张，缺省 1）。
	Cap int
	// Tag 展示标签（被动修饰），空则无被动。
	Tag string
}

var _ interface {
	item.Def
	item.Equippable
	item.EquipGated
	item.RelationGated
	item.Passive
} = (*RelationCard)(nil)

// EquipSlot 返回关系卡槽。
func (c *RelationCard) EquipSlot() item.Slot { return item.SlotRelation }

// Capacity 返回槽容量，默认 1。
func (c *RelationCard) Capacity() int {
	if c.Cap <= 0 {
		return 1
	}
	return c.Cap
}

// RequiresRelation 返回依赖的关系类型。
func (c *RelationCard) RequiresRelation() relation.Type { return c.Requires }

// CanEquip 检查玩家是否存在对应类型的生效关系。
func (c *RelationCard) CanEquip(ctx context.Context, inv *item.Inventory, owner string) (bool, error) {
	if inv.Relations == nil {
		return false, nil
	}
	_, err := inv.Relations.FindActive(ctx, owner, c.Requires)
	return err == nil, nil
}

// Modifiers 返回展示标签被动。
func (c *RelationCard) Modifiers() []item.Modifier {
	if c.Tag == "" {
		return nil
	}
	return []item.Modifier{{Key: "mentor_tag", Value: c.Tag}}
}

// RelationCards 关系卡示例集：师徒卡（需师徒关系）、闺蜜卡（需闺蜜关系，可并穿 2）。
func RelationCards() []item.Def {
	return []item.Def{
		&RelationCard{
			Common:   item.Common{ID: "card_master", Category: item.CategoryRelationCard, Name: "Master Card", Priority: 50, Rarity: 3},
			Requires: relation.TypeMaster,
			Tag:      "master",
		},
		&RelationCard{
			Common:   item.Common{ID: "card_bestie", Category: item.CategoryRelationCard, Name: "Bestie Card", Priority: 50, Rarity: 3},
			Requires: relation.TypeBestie,
			Cap:      2,
		},
	}
}

// ---------------------------------------------------------------------------
// 飘屏卡：可堆叠消耗品，使用时需用户输入文案（或用预置文案）。

// BannerCard 飘屏卡道具类型。
type BannerCard struct {
	item.Common
	// Duration 飘屏展示时长。
	Duration time.Duration
	// TextParam 用户输入文案的参数键，默认 "text"。
	TextParam string
	// DefaultText 预置文案；非空时不要求用户输入。
	DefaultText string
	// Banner 广播端口。
	Banner BannerBroadcaster
}

var _ interface {
	item.Def
	item.Usable
} = (*BannerCard)(nil)

// Use 校验文案（缺参数在副作用前失败）并广播飘屏。
func (b *BannerCard) Use(ctx context.Context, inv *item.Inventory, owner string, params map[string]any) error {
	if b.Banner == nil {
		return item.ErrNotConfigured
	}
	text := b.DefaultText
	if text == "" {
		key := b.TextParam
		if key == "" {
			key = "text"
		}
		text, _ = params[key].(string)
	}
	if text == "" {
		return item.ErrMissingParam
	}
	return b.Banner.Broadcast(ctx, owner, text, b.Duration)
}

// Banners 飘屏卡示例集：玫瑰（10s）、烈焰（30s），均 99 堆叠。
func Banners(banner BannerBroadcaster) []item.Def {
	return []item.Def{
		&BannerCard{
			Common:   item.Common{ID: "banner_rose", Category: item.CategoryConsumable, Name: "Rose Banner", Rarity: 2, MaxStack: 99},
			Duration: 10 * time.Second,
			Banner:   banner,
		},
		&BannerCard{
			Common:   item.Common{ID: "banner_fire_30s", Category: item.CategoryConsumable, Name: "Fire Banner (30s)", Rarity: 4, MaxStack: 99},
			Duration: 30 * time.Second,
			Banner:   banner,
		},
	}
}

// ---------------------------------------------------------------------------
// 聊天气泡：气泡槽独占；限时款到期仅卸下保留。

// ChatBubble 聊天气泡道具类型。
type ChatBubble struct {
	item.Common
	// Duration 有效期，0 表示永久。
	Duration time.Duration
	// Policy 过期策略，空视为 unequip。
	Policy string
}

var _ interface {
	item.Def
	item.Equippable
	item.Expirable
} = (*ChatBubble)(nil)

// EquipSlot 返回气泡槽。
func (b *ChatBubble) EquipSlot() item.Slot { return item.SlotChatBubble }

// Capacity 气泡槽独占。
func (b *ChatBubble) Capacity() int { return 1 }

// Lifetime 限时款的有效期。
func (b *ChatBubble) Lifetime() time.Duration { return b.Duration }

// ExpirePolicy 过期策略，默认仅卸下保留。
func (b *ChatBubble) ExpirePolicy() string {
	if b.Policy == "" {
		return item.PolicyUnequip
	}
	return b.Policy
}

// DowngradeTo 气泡不支持降级。
func (b *ChatBubble) DowngradeTo() string { return "" }

// ChatBubbles 气泡示例集：星耀（永久）、极光（7天，到期卸下）。
func ChatBubbles() []item.Def {
	return []item.Def{
		&ChatBubble{Common: item.Common{ID: "bubble_star", Category: item.CategoryChatBubble, Name: "Star Bubble", Priority: 45, Rarity: 3}},
		&ChatBubble{
			Common:   item.Common{ID: "bubble_aurora", Category: item.CategoryChatBubble, Name: "Aurora Bubble", Priority: 75, Rarity: 5},
			Duration: 7 * 24 * time.Hour,
			Policy:   item.PolicyUnequip,
		},
	}
}

// ---------------------------------------------------------------------------
// 铭牌：铭牌槽独占；节日款到期即删。

// Nameplate 铭牌道具类型。
type Nameplate struct {
	item.Common
	// Color 铭牌配色（被动修饰），空则无被动。
	Color string
	// Duration 有效期，0 表示永久。
	Duration time.Duration
	// Policy 过期策略，空视为 remove。
	Policy string
}

var _ interface {
	item.Def
	item.Equippable
	item.Passive
	item.Expirable
} = (*Nameplate)(nil)

// EquipSlot 返回铭牌槽。
func (n *Nameplate) EquipSlot() item.Slot { return item.SlotNamePlate }

// Capacity 铭牌槽独占。
func (n *Nameplate) Capacity() int { return 1 }

// Modifiers 返回配色被动。
func (n *Nameplate) Modifiers() []item.Modifier {
	if n.Color == "" {
		return nil
	}
	return []item.Modifier{{Key: "plate_color", Value: n.Color}}
}

// Lifetime 限时款的有效期。
func (n *Nameplate) Lifetime() time.Duration { return n.Duration }

// ExpirePolicy 过期策略，默认删除。
func (n *Nameplate) ExpirePolicy() string {
	if n.Policy == "" {
		return item.PolicyRemove
	}
	return n.Policy
}

// DowngradeTo 铭牌不支持降级。
func (n *Nameplate) DowngradeTo() string { return "" }

// Nameplates 铭牌示例集：余烬（永久+配色）、节日限定（72小时，到期删除）。
func Nameplates() []item.Def {
	return []item.Def{
		&Nameplate{
			Common: item.Common{ID: "np_ember", Category: item.CategoryNamePlate, Name: "Ember Plate", Priority: 80, Rarity: 4},
			Color:  "ember",
		},
		&Nameplate{
			Common:   item.Common{ID: "np_festival_72h", Category: item.CategoryNamePlate, Name: "Festival Plate (72h)", Priority: 82, Rarity: 5},
			Duration: 72 * time.Hour,
			Policy:   item.PolicyRemove,
		},
	}
}

// ---------------------------------------------------------------------------
// CP 戒指：拾取即绑定，穿戴前置为生效 CP 关系；BindCP 流程成对发放并自动穿戴。

// CPRing CP 戒指道具类型。
type CPRing struct {
	item.Common
	// Badge CP 徽记样式（被动修饰），空则无被动。
	Badge string
	// Duration 有效期，0 表示永久。
	Duration time.Duration
	// Policy 过期策略，空视为 remove。
	Policy string
}

var _ interface {
	item.Def
	item.Equippable
	item.EquipGated
	item.RelationGated
	item.Passive
	item.Expirable
} = (*CPRing)(nil)

// EquipSlot 返回 CP 戒指槽。
func (r *CPRing) EquipSlot() item.Slot { return item.SlotCPRing }

// Capacity 戒指槽独占。
func (r *CPRing) Capacity() int { return 1 }

// RequiresRelation 戒指依赖 CP 关系。
func (r *CPRing) RequiresRelation() relation.Type { return relation.TypeCP }

// CanEquip 检查玩家是否存在生效 CP 关系。
func (r *CPRing) CanEquip(ctx context.Context, inv *item.Inventory, owner string) (bool, error) {
	if inv.Relations == nil {
		return false, nil
	}
	_, err := inv.Relations.FindActive(ctx, owner, relation.TypeCP)
	return err == nil, nil
}

// Modifiers 返回 CP 徽记被动。
func (r *CPRing) Modifiers() []item.Modifier {
	if r.Badge == "" {
		return nil
	}
	return []item.Modifier{{Key: "cp_badge", Value: r.Badge}}
}

// Lifetime 限时款的有效期。
func (r *CPRing) Lifetime() time.Duration { return r.Duration }

// ExpirePolicy 过期策略，默认删除。
func (r *CPRing) ExpirePolicy() string {
	if r.Policy == "" {
		return item.PolicyRemove
	}
	return r.Policy
}

// DowngradeTo 戒指不支持降级。
func (r *CPRing) DowngradeTo() string { return "" }

// CPRings 戒指示例集：钻石（永久、绑定）、黄金（30天、绑定、到期删除）。
func CPRings() []item.Def {
	return []item.Def{
		&CPRing{
			Common: item.Common{ID: "ring_cp_diamond", Category: item.CategoryCPRing, Name: "CP Diamond Ring", Priority: 95, Rarity: 5, Bind: true},
			Badge:  "diamond",
		},
		&CPRing{
			Common:   item.Common{ID: "ring_cp_gold", Category: item.CategoryCPRing, Name: "CP Gold Ring (30d)", Priority: 78, Rarity: 3, Bind: true},
			Duration: 30 * 24 * time.Hour,
			Policy:   item.PolicyRemove,
		},
	}
}

// ---------------------------------------------------------------------------
// 头像框：头像槽独占 + 被动配色（演示最简单的穿戴道具）。

// AvatarFrame 头像框道具类型。
type AvatarFrame struct {
	item.Common
	// FrameColor 框色（被动修饰）。
	FrameColor string
}

var _ interface {
	item.Def
	item.Equippable
	item.Passive
} = (*AvatarFrame)(nil)

// EquipSlot 返回头像槽。
func (f *AvatarFrame) EquipSlot() item.Slot { return item.SlotAvatar }

// Capacity 头像槽独占。
func (f *AvatarFrame) Capacity() int { return 1 }

// Modifiers 返回框色被动。
func (f *AvatarFrame) Modifiers() []item.Modifier {
	if f.FrameColor == "" {
		return nil
	}
	return []item.Modifier{{Key: "frame_color", Value: f.FrameColor}}
}

// AvatarFrames 头像框示例集：金色头像框。
func AvatarFrames() []item.Def {
	return []item.Def{
		&AvatarFrame{
			Common:     item.Common{ID: "avatar_gold", Category: item.CategoryAvatar, Name: "Gold Avatar", Priority: 90, Rarity: 5},
			FrameColor: "gold",
		},
	}
}

// ---------------------------------------------------------------------------
// 改名卡：使用时由用户输入新名字，直接调用用户服务改名。
// 这是"道具行为写在类型方法里"的最直接示范。

// RenameCard 改名卡道具类型。
type RenameCard struct {
	item.Common
	// Users 用户服务端口，Use 时调用其改名。
	Users RenameService
}

var _ interface {
	item.Def
	item.Usable
} = (*RenameCard)(nil)

// Use 校验新名字（缺参数在副作用前失败）并调用用户服务改名；
// 扣减由通用层在使用成功后完成。
func (c *RenameCard) Use(ctx context.Context, inv *item.Inventory, owner string, params map[string]any) error {
	if c.Users == nil {
		return item.ErrNotConfigured
	}
	newName, _ := params["new_name"].(string)
	if newName == "" {
		return item.ErrMissingParam
	}
	return c.Users.Rename(ctx, owner, newName)
}

// RenameCards 改名卡示例集：标准改名卡（99 堆叠）。
func RenameCards(users RenameService) []item.Def {
	return []item.Def{
		&RenameCard{
			Common: item.Common{ID: "rename_card", Category: item.CategoryConsumable, Name: "Rename Card", Rarity: 2, MaxStack: 99},
			Users:  users,
		},
	}
}

// ---------------------------------------------------------------------------
// 药水：使用即给账本入账（演示数值型 Usable 道具）。

// Potion 药水道具类型。
type Potion struct {
	item.Common
	// Currency 入账币种。
	Currency string
	// Amount 单次使用入账数额。
	Amount int64
	// Ledger 账本端口。
	Ledger Ledger
}

var _ interface {
	item.Def
	item.Usable
} = (*Potion)(nil)

// Use 给玩家入账一次；扣减由通用层完成。
func (p *Potion) Use(ctx context.Context, inv *item.Inventory, owner string, params map[string]any) error {
	if p.Ledger == nil {
		return item.ErrNotConfigured
	}
	return p.Ledger.Add(ctx, owner, p.Currency, p.Amount)
}

// Potions 药水示例集：HP 药水（+50 hp，99 堆叠）。
func Potions(ledger Ledger) []item.Def {
	return []item.Def{
		&Potion{
			Common:   item.Common{ID: "potion_hp", Category: item.CategoryConsumable, Name: "HP Potion", Rarity: 1, MaxStack: 99},
			Currency: "hp",
			Amount:   50,
			Ledger:   ledger,
		},
	}
}

// ---------------------------------------------------------------------------
// 宝箱：使用时按权重随机再发放（演示依赖 inv.Grant 的 Usable 道具）。

// ChestEntry 宝箱的一个候选条目。
type ChestEntry struct {
	// DefID 命中后发放的道具标识。
	DefID string
	// Count 发放数量。
	Count int64
	// Weight 抽取权重。
	Weight int64
}

// Chest 宝箱道具类型。
type Chest struct {
	item.Common
	// Entries 候选条目，按权重随机命中其一。
	Entries []ChestEntry
	// Rand 随机源：入参为总权重，返回 [0, n) 内的随机数；测试可注入固定值。
	Rand func(n int64) int64
}

var _ interface {
	item.Def
	item.Usable
} = (*Chest)(nil)

// Use 加权随机命中一个条目并调用通用层发放。
func (c *Chest) Use(ctx context.Context, inv *item.Inventory, owner string, params map[string]any) error {
	entry, ok := c.pick()
	if !ok {
		return nil
	}
	_, err := inv.Grant(ctx, item.GrantRequest{
		Owner:  owner,
		DefID:  entry.DefID,
		Count:  entry.Count,
		Source: "effect",
		Reason: "chest",
	})
	return err
}

// pick 加权随机抽取：在 [0, total) 区间按权重顺序命中条目。
func (c *Chest) pick() (ChestEntry, bool) {
	if len(c.Entries) == 0 || c.Rand == nil {
		return ChestEntry{}, false
	}
	var total int64
	for _, e := range c.Entries {
		total += e.Weight
	}
	if total <= 0 {
		return ChestEntry{}, false
	}
	n := c.Rand(total)
	for _, e := range c.Entries {
		if n < e.Weight {
			return e, true
		}
		n -= e.Weight
	}
	// 理论不可达，兜底返回最后一个条目。
	return c.Entries[len(c.Entries)-1], true
}

// Chests 宝箱示例集：基础宝箱（70% 出 3 瓶 HP 药水，30% 出烈焰勋章）。
func Chests(rand func(n int64) int64) []item.Def {
	return []item.Def{
		&Chest{
			Common: item.Common{ID: "chest_basic", Category: item.CategoryConsumable, Name: "Basic Chest", Rarity: 2},
			Entries: []ChestEntry{
				{DefID: "potion_hp", Count: 3, Weight: 70},
				{DefID: "badge_flame", Count: 1, Weight: 30},
			},
			Rand: rand,
		},
	}
}
