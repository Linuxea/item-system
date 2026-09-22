package items

import (
	"time"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/port"
)

// Deps 构建道具册所需的全部外部依赖。
//
// 这是整个体系里唯一一处「全量依赖」出现的地方 —— 装配点。
// 每个道具在构造时只拿走自己需要的那一两个，之后运行期不再有任何依赖穿越调用。
type Deps struct {
	// Users 用户服务，改名卡需要。
	Users port.UserService
	// Ledger 货币账本，货币包需要。
	Ledger port.Ledger
	// Levels 等级来源，带等级门槛的道具需要。
	Levels port.LevelSource
	// Broadcaster 广播网关，飘屏卡需要。
	Broadcaster port.Broadcaster
	// Granter 发放器，宝箱需要；通常传 engine.NewLateGranter(...)。
	Granter port.Granter
	// Rand 随机数来源，宝箱需要；测试时注入固定实现。
	Rand port.Rand
	// Relations 关系查询，关系卡与 CP 戒指需要。
	Relations port.RelationChecker
}

const (
	day  = 24 * time.Hour
	week = 7 * day
)

// Catalog 返回全部内置道具。
//
// 一百款座驾就是一百行 NewMount —— 参数是 struct 字段，拼错编译不过，
// IDE 能补全、能跳转、能重构。这和「一百行配置 map」的关键区别就在这里。
func Catalog(d Deps) []item.Item {
	all := []item.Item{}
	all = append(all, mounts(d)...)
	all = append(all, badges()...)
	all = append(all, headwear()...)
	all = append(all, vips()...)
	all = append(all, nameplates()...)
	all = append(all, chatBubbles()...)
	all = append(all, relationItems(d)...)
	all = append(all, consumables(d)...)
	return all
}

// mounts 座驾：永久款两种，限时款到期即删。
func mounts(d Deps) []item.Item {
	return []item.Item{
		NewMount("mount_cloud", "祥云座驾", 65, 3, 80, 1, d.Levels, 0, ""),
		NewMount("mount_dragon", "游龙战车", 85, 5, 150, 30, d.Levels, 0, ""),
		NewMount("mount_ember_7d", "炽焰飞车（7 天）", 70, 4, 120, 10, d.Levels, week, item.ExpireRemove),
	}
}

// badges 勋章：badge 槽容量 3，可同时佩戴多枚。
func badges() []item.Item {
	return []item.Item{
		NewBadge("badge_abyss", "深渊征服者", 70, 5),
		NewBadge("badge_flame", "烈焰老兵", 70, 3),
		NewBadge("badge_rookie", "新星勋章", 40, 1),
	}
}

// headwear 头饰：限时款到期仅卸下，实例保留在背包。
func headwear() []item.Item {
	return []item.Item{
		NewHeadwear("crown_aurora", "极光之冠", 88, 5, "aurora", 0, ""),
		NewHeadwear("cap_sprint_3d", "疾风帽（3 天）", 55, 2, "sprint", 3*day, item.ExpireUnequip),
	}
}

// vips 会员：月卡到期降周卡，周卡到期降体验卡，体验卡到期删除 —— 降级链。
// 每一档只知道自己的下一档是谁，没有任何一处保存整条链。
func vips() []item.Item {
	return []item.Item{
		NewVIP("vip_month", "月卡会员", 95, 5, 3, 30*day, "vip_week"),
		NewVIP("vip_week", "周卡会员", 90, 3, 1, week, "vip_trial"),
		NewVIP("vip_trial", "体验会员", 80, 1, 0, 3*day, ""),
	}
}

// nameplates 铭牌。
func nameplates() []item.Item {
	return []item.Item{
		NewNamePlate("plate_glory", "荣耀铭牌", 75, 4, 0, ""),
		NewNamePlate("plate_season_30d", "赛季铭牌（30 天）", 60, 3, 30*day, item.ExpireRemove),
	}
}

// chatBubbles 聊天气泡。
func chatBubbles() []item.Item {
	return []item.Item{
		NewChatBubble("bubble_sakura", "樱花气泡", 60, 3, "sakura"),
		NewChatBubble("bubble_starry", "星河气泡", 80, 5, "starry"),
	}
}

// relationItems 关系卡与 CP 戒指：都需要先建立关系才能穿戴。
func relationItems(d Deps) []item.Item {
	return []item.Item{
		NewRelationCard("card_bestie", "挚友卡", 72, 4, "bestie", "挚友", d.Relations, 0, ""),
		NewRelationCard("card_master_30d", "师徒卡（30 天）", 68, 3, "master", "师徒", d.Relations, 30*day, item.ExpireRemove),
		NewCPRing("ring_eternal", "永恒之戒", 92, 5, 100, d.Relations),
	}
}

// consumables 消耗品。
func consumables(d Deps) []item.Item {
	return []item.Item{
		NewRenameCard(d.Users),
		NewBannerCard("banner_rose", "玫瑰飘屏", 4, 8, d.Broadcaster),
		NewCurrencyPack("pack_gold_100", "金币袋（100）", "gold", 100, d.Ledger),
		NewTreasureChest("chest_starter", "新手宝箱", 3, []ChestEntry{
			{ItemID: "pack_gold_100", Count: 1, Weight: 60},
			{ItemID: "badge_rookie", Count: 1, Weight: 30},
			{ItemID: "mount_cloud", Count: 1, Weight: 10},
		}, d.Granter, d.Rand),
	}
}
