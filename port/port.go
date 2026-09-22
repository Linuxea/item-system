// Package port 声明道具体系依赖的外部能力（端口）。
//
// 依赖倒置：接口由本体系声明，具体实现由外部提供（真实项目里是用户服务、
// 货币账本、推送网关等），在装配点注入给需要它的道具。
// 道具只持有自己那一个端口，不存在「什么都能拿到的工具箱」。
package port

import "context"

// UserService 用户资料服务：改名卡等道具依赖。
type UserService interface {
	// Rename 修改玩家昵称。
	Rename(ctx context.Context, owner, newName string) error
}

// Ledger 货币账本：货币包等道具依赖。
type Ledger interface {
	// Add 给玩家增加指定货币。
	Add(ctx context.Context, owner, currency string, amount int64) error
}

// LevelSource 玩家等级来源：带等级门槛的道具依赖。
type LevelSource interface {
	// LevelOf 返回玩家当前等级。
	LevelOf(ctx context.Context, owner string) (int64, error)
}

// Broadcaster 全服广播网关：飘屏卡依赖。
type Broadcaster interface {
	// Broadcast 向全服广播一条来自 owner 的消息。
	Broadcast(ctx context.Context, owner, text string, seconds int) error
}

// Granter 发放器：宝箱、礼包等「用了之后再发别的道具」的道具依赖。
// 由引擎实现并在装配点回注，打破「道具 -> 引擎 -> 道具」的循环。
type Granter interface {
	// Grant 给玩家发放指定道具。
	Grant(ctx context.Context, owner, itemID string, count int64) error
}

// RelationChecker 关系查询：关系卡、CP 戒指的穿戴前置条件依赖。
type RelationChecker interface {
	// HasActive 报告玩家当前是否存在指定类型的有效关系。
	HasActive(ctx context.Context, owner, relationType string) (bool, error)
}

// Rand 随机数来源：随机宝箱依赖。测试时注入固定实现以获得确定性。
type Rand func(n int64) int64
