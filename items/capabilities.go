package items

import "github.com/Linuxea/item-system/item"

// 本文件是一张「哪个道具有哪些能力」的对照表。
//
// 它取代了旧版模板里的 Behaviors 配置 map，区别在于：
// 这些断言由**编译器**检查。漏写一个方法、方法签名写错、把能力配错道具，
// 全都是编译错误，不会等到运行时才炸。
//
// 新增道具时在这里补上它的能力行；能力没实现完整的话，go build 就过不去。

// ---- 座驾：穿戴 / 被动 / 等级门槛 / 展示 / 时效 ----
var (
	_ item.Item        = (*Mount)(nil)
	_ item.Equippable  = (*Mount)(nil)
	_ item.Passive     = (*Mount)(nil)
	_ item.Conditional = (*Mount)(nil)
	_ item.Displayable = (*Mount)(nil)
	_ item.Expirable   = (*Mount)(nil)
)

// ---- 头饰：穿戴 / 被动 / 展示 / 时效 ----
var (
	_ item.Item        = (*Headwear)(nil)
	_ item.Equippable  = (*Headwear)(nil)
	_ item.Passive     = (*Headwear)(nil)
	_ item.Displayable = (*Headwear)(nil)
	_ item.Expirable   = (*Headwear)(nil)
)

// ---- 勋章：穿戴（容量 3）/ 展示。刻意没有 Passive ----
var (
	_ item.Item        = (*Badge)(nil)
	_ item.Equippable  = (*Badge)(nil)
	_ item.Displayable = (*Badge)(nil)
)

// ---- 铭牌 / 头像框：穿戴 / 展示 / 时效 ----
var (
	_ item.Item        = (*NamePlate)(nil)
	_ item.Equippable  = (*NamePlate)(nil)
	_ item.Displayable = (*NamePlate)(nil)
	_ item.Expirable   = (*NamePlate)(nil)

	_ item.Item        = (*Avatar)(nil)
	_ item.Equippable  = (*Avatar)(nil)
	_ item.Displayable = (*Avatar)(nil)
	_ item.Expirable   = (*Avatar)(nil)
)

// ---- 聊天气泡：穿戴 / 被动 / 展示 ----
var (
	_ item.Item        = (*ChatBubble)(nil)
	_ item.Equippable  = (*ChatBubble)(nil)
	_ item.Passive     = (*ChatBubble)(nil)
	_ item.Displayable = (*ChatBubble)(nil)
)

// ---- VIP：穿戴 / 被动 / 展示 / 时效 / 降级 ----
var (
	_ item.Item         = (*VIP)(nil)
	_ item.Equippable   = (*VIP)(nil)
	_ item.Passive      = (*VIP)(nil)
	_ item.Displayable  = (*VIP)(nil)
	_ item.Expirable    = (*VIP)(nil)
	_ item.Downgradable = (*VIP)(nil)
)

// ---- 关系卡：穿戴 / 被动 / 关系门槛 / 展示 / 时效 ----
var (
	_ item.Item        = (*RelationCard)(nil)
	_ item.Equippable  = (*RelationCard)(nil)
	_ item.Passive     = (*RelationCard)(nil)
	_ item.Conditional = (*RelationCard)(nil)
	_ item.Displayable = (*RelationCard)(nil)
	_ item.Expirable   = (*RelationCard)(nil)
)

// ---- CP 戒指：穿戴 / 被动 / 关系门槛 / 展示 / 发放即绑定 ----
var (
	_ item.Item        = (*CPRing)(nil)
	_ item.Equippable  = (*CPRing)(nil)
	_ item.Passive     = (*CPRing)(nil)
	_ item.Conditional = (*CPRing)(nil)
	_ item.Displayable = (*CPRing)(nil)
	_ item.Bindable    = (*CPRing)(nil)
)

// ---- 改名卡：使用 / 需要参数 / 堆叠。刻意没有 Equippable ----
var (
	_ item.Item          = (*RenameCard)(nil)
	_ item.Usable        = (*RenameCard)(nil)
	_ item.Parameterized = (*RenameCard)(nil)
	_ item.Stackable     = (*RenameCard)(nil)
)

// ---- 飘屏卡：使用 / 需要参数 / 堆叠 / 展示 ----
var (
	_ item.Item          = (*BannerCard)(nil)
	_ item.Usable        = (*BannerCard)(nil)
	_ item.Parameterized = (*BannerCard)(nil)
	_ item.Stackable     = (*BannerCard)(nil)
	_ item.Displayable   = (*BannerCard)(nil)
)

// ---- 货币包：使用 / 堆叠。不需要参数，所以没有 Parameterized ----
var (
	_ item.Item      = (*CurrencyPack)(nil)
	_ item.Usable    = (*CurrencyPack)(nil)
	_ item.Stackable = (*CurrencyPack)(nil)
)

// ---- 随机宝箱：使用 / 堆叠 / 展示 ----
var (
	_ item.Item        = (*TreasureChest)(nil)
	_ item.Usable      = (*TreasureChest)(nil)
	_ item.Stackable   = (*TreasureChest)(nil)
	_ item.Displayable = (*TreasureChest)(nil)
)
