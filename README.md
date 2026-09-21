# item-system

Go 实现的道具子系统库（`github.com/Linuxea/item-system`）。面向游戏 / 社交业务中的虚拟物品场景：发放、使用、穿戴、时效、双主体关系与展示快照。

设计原则：**组合优于继承**。新增道具类型 = 配置数据 + 既有行为组件的组合，核心零改动。

## 特性

- **模板 / 实例分离**：规则在模板（只读配置），状态在实例（运行时数据，乐观锁版本号）
- **行为组件**：Stackable / Expirable / Equippable / Usable / Bindable / Passive / Condition，按需组合
- **效果命令**：使用道具产出 `effect.Command`（加货币、再发放、随机宝箱、全服飘屏等），命令自带依赖、物化后才执行
- **发放语义**：堆叠、时效、幂等键防重
- **穿戴与展示**：槽位 + 容量、多级排序、每场景一个 SortPolicy
- **过期策略**：remove / unequip / downgrade（支持 vip3 → vip1 → vip0 降级链）
- **双主体关系**：关系卡、CP 戒指为独立聚合根；解除关系联动卸下相关道具
- **领域事件**：`item.granted / consumed / equipped / unequipped / expired`
- **可插拔存储**：领域侧只声明端口，`infrastructure/memory` 提供完整内存实现（亦作 MySQL 实现的对照）

## 架构

```
interfaces（未来：gRPC/GM 命令）
    │
application ──────────── 装配点（唯一允许出现"全量依赖"的地方）
    │
domain（纯领域，零基础设施依赖）
    ├── model        模板/实例/槽位/修饰符 —— 纯数据
    ├── behavior     行为组件 + Registry（配置→组件）
    ├── effect       效果命令（自带依赖、自带物化规则、自带 Exec）
    ├── grant        发放（堆叠/时效/幂等）
    ├── usage        主动使用（物化→执行→扣减）
    ├── profile      穿戴 + 展示快照 + 场景排序
    ├── expiry       过期策略
    ├── relation     双主体关系（独立聚合根）
    └── event        领域事件
    │
infrastructure ──────── memory（完整内存实现）
    │
catalog ─────────────── 9 类道具模板示例集（纯配置数据）
```

关键约束（编译器强制，非纪律约束）：

- `domain/` 生产代码不 import `application` / `infrastructure` / `catalog`
- 每个领域服务在自己包内声明**消费侧窄接口**（如 `grant.instanceStore` 只见 `ListByOwner / Save / Update`），提供方以结构化类型自动满足
- 全量依赖（`application.Deps` / `behavior.Ports`）只在启动装配点存在一次，热路径只携带 `(ctx, owner)`
- 用户输入止步于 `effect.Materialize`（唯一消费 `Params` 的地方）；命令出厂即完整，缺参数在任何副作用前失败

## 快速开始

```go
package main

import (
	"context"
	"fmt"

	"github.com/Linuxea/item-system/catalog"
	"github.com/Linuxea/item-system/domain/grant"
	"github.com/Linuxea/item-system/domain/usage"
	"github.com/Linuxea/item-system/infrastructure/memory"
)

func main() {
	stack := memory.NewStack(catalog.All()...)
	app := stack.App
	ctx := context.Background()

	// 发放座驾并穿上
	res, err := app.Grant(ctx, grant.Request{
		Owner: "u1", TemplateID: "mount_cloud", Count: 1,
		Source: "gm", Reason: "login_gift",
	})
	if err != nil {
		panic(err)
	}
	if err := app.Equip(ctx, "u1", res.InstanceID); err != nil {
		panic(err)
	}

	// 使用飘屏（用户输入的文案经 Params 物化进命令）
	grantRes, _ := app.Grant(ctx, grant.Request{
		Owner: "u1", TemplateID: "banner_rose", Count: 1,
		Source: "shop", Reason: "purchase",
	})
	_, err = app.Use(ctx, usage.Request{
		Owner: "u1", InstanceID: grantRes.InstanceID, Count: 1,
		Params: map[string]any{"text": "Hello!"},
	})
	if err != nil {
		panic(err)
	}

	// 构建聊天场景展示快照
	snap, err := app.BuildProfile(ctx, "u1", "chat")
	if err != nil {
		panic(err)
	}
	fmt.Println(snap.Owner, snap.Slots)
}
```

自定义时钟 / RNG 等依赖时，手动组装 `application.Deps`（模式见 `application/mount_test.go:16`）。

## 核心概念

| 概念 | 说明 |
|---|---|
| `model.ItemTemplate` | 道具规则：分类、优先级、稀有度、`Behaviors` 配置（`map[key]config`） |
| `model.ItemInstance` | 玩家持有的实例：数量、绑定、状态、过期时间、版本号 |
| 行为组件 | `behavior` 包内的 7 个组件，由 `Registry` 从模板配置编译 |
| `effect.Command` | 两方法接口（`Kind` / `Exec`），每个命令私有持有一个端口 |
| 槽位 | avatar / nameplate / badge / vip / mount / headwear / relation / cp_ring / chat_bubble |
| 场景快照 | `profile.Snapshot`：按槽位聚合已穿戴道具 + 合并修饰符，按场景 SortPolicy 排序 |
| 关系 | `relation.Relation`：双方主体、类型、可选时效；过期/解除时联动卸下 |

## 扩展指引

- **新道具类型**：在 `catalog/` 组合既有行为（必要时在 `domain/model` 加槽位常量），不改核心
- **新效果**：新增 Command struct（私有端口 + 构造器 + `Exec`）+ `behavior.decodeEffect` 一个 case + `behavior.Ports` 一个字段（如需新端口）
- **真实存储**：按 `domain/repository` 提供方接口实现 MySQL（模板表/实例表/条件更新）、Redis（背包缓存/过期 ZSet 调度）

## 目录结构

```
application/      装配点：Deps、App、grantAdapter（打破 Grant ⇄ Registry 循环）
catalog/          9 类道具模板示例（座驾/勋章/头饰/VIP/关系卡/飘屏/气泡/铭牌/CP 戒指）
domain/
  behavior/       行为组件、Registry、Ports
  effect/         效果命令与 Materialize
  event/          领域事件
  expiry/         过期策略执行
  grant/ usage/   发放与使用
  model/          纯数据：模板/实例/槽位/修饰符
  profile/        穿戴与场景快照
  relation/       双主体关系
  repository/     提供方接口与错误类型
docs/DESIGN.md    设计演进全文
infrastructure/memory/  内存实现：仓储、事件总线、账本、完整 Stack
```

## 验证

```sh
gofmt -w . && go vet ./... && go test -race ./...
```

Race 模式必需：仓储与事件总线会被并发访问。

## 设计文档

架构决策、接口演进四步曲与代价复盘见 [docs/DESIGN.md](docs/DESIGN.md)。

## 已知边界

- 存储仅有内存实现；MySQL / Redis 按提供方接口接入
- 对方同意流程（关系建立）、VIP 每日定时特权、座驾进阶养成线未纳入 v1
