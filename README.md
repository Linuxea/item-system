# item-system

Go 实现的道具子系统库（`github.com/Linuxea/item-system`）。面向游戏 / 社交业务中的虚拟物品场景：发放、使用、穿戴、时效、双主体关系与展示快照。

设计原则：**显式类型 + 面向对象多态**。每种道具是一个 Go 类型，行为写在类型的方法里（改名卡的 `Use` 直接调用用户服务）；通用库存层（`item.Inventory`）只做与道具种类无关的事，通过可选接口的类型断言发现道具能力。

## 特性

- **模板 / 实例分离**：规则在道具定义（代码构造的 Go 对象），状态在实例（运行时数据，乐观锁版本号）
- **每类道具一个显式类型**：内嵌 `item.Common` 获得展示标签，按需实现 `Usable` / `Equippable` / `Expirable` / `Passive` / `EquipGated` / `RelationGated`
- **发放语义**：堆叠（同时效窗口合并）、时效、绑定、幂等键防重
- **使用语义**：通用层校验并扣减；业务效果在道具的 `Use` 方法里，失败零消耗
- **穿戴与展示**：槽位 + 容量、前置条件道具自判、多级排序、每场景一个 SortPolicy
- **过期策略**：remove / unequip / downgrade（支持 vip3 → vip1 → vip0 降级链，降级继承目标时效）
- **双主体关系**：关系卡、CP 戒指；解除/到期联动卸下相关道具；BindCP 成对发放并自动穿戴
- **领域事件**：`item.granted / consumed / equipped / unequipped / expired` + `relation.bound / dissolved`
- **可插拔存储**：核心只声明端口接口，`memory` 提供完整内存实现（亦作 MySQL/Redis 实现的对照）

## 架构

```
event/       事件接口 + Publisher
relation/    双主体关系：模型 + Repo 接口 + 关系事件
item/
  def.go         Def 接口 + Common + 可选能力接口
  model.go       实例/槽位/状态/修饰符/穿戴记录
  events.go      领域事件
  inventory.go   端口 + Inventory：Grant / Use
  equip.go       Equip / Unequip / 快照 + 场景排序
  expiry.go      三种过期策略
  relations.go   关系流程 + BindCP + 解除联动
  catalog/       具体道具类型（座驾/勋章/头饰/VIP/关系卡/飘屏/气泡/铭牌/CP戒指/头像框/改名卡/药水/宝箱）
memory/      全套内存实现 + DefRegistry + Stack
```

## 快速开始

```go
package main

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/item/catalog"
	"github.com/Linuxea/item-system/memory"
)

func main() {
	stack := memory.NewStack()
	// 先建栈，再注册依赖其组件的道具定义（等级源/广播端口/账本/随机源）
	stack.Defs.Register(catalog.Mounts(stack.Levels)...)
	stack.Defs.Register(catalog.Banners(stack.Banner)...)
	stack.Defs.Register(catalog.RenameCards(stack.Renamer)...)
	stack.Defs.Register(catalog.Potions(stack.Ledger)...)
	stack.Defs.Register(catalog.Chests(rand.Int64N)...)

	inv := stack.Inv
	ctx := context.Background()

	// 发放座驾并穿上（等级不足会得到 ErrConditionNotMet）
	stack.Levels.SetLevel("u1", 30)
	res, err := inv.Grant(ctx, item.GrantRequest{
		Owner: "u1", DefID: "mount_dragon", Count: 1,
		Source: "gm", Reason: "login_gift",
	})
	if err != nil {
		panic(err)
	}
	if err := inv.Equip(ctx, "u1", res.InstanceID); err != nil {
		panic(err)
	}

	// 使用改名卡（业务逻辑在 RenameCard.Use 里，扣减由通用层完成）
	card, _ := inv.Grant(ctx, item.GrantRequest{Owner: "u1", DefID: "rename_card", Count: 1})
	if _, err := inv.Use(ctx, item.UseRequest{
		Owner: "u1", InstanceID: card.InstanceID,
		Params: map[string]any{"new_name": "风暴之灵"},
	}); err != nil {
		panic(err)
	}

	// 构建展示快照
	snap, err := inv.Build(ctx, "u1", "chat")
	if err != nil {
		panic(err)
	}
	fmt.Println(snap.Owner, snap.Slots)
}
```

自定义时钟 / ID 生成：装配后直接替换 `stack.Inv.Now` / `stack.Inv.NewID`（导出字段）。

## 新增一种道具

```go
// 1. 定义类型：内嵌 Common，声明需要的依赖与数据
type Firework struct {
	item.Common
	Burst  time.Duration
	Banner BannerBroadcaster
}

// 2. 实现想要的能力接口（不想要的就不实现）
func (f *Firework) Use(ctx context.Context, inv *item.Inventory, owner string, params map[string]any) error {
	return f.Banner.Broadcast(ctx, owner, "烟花绽放！", f.Burst)
}

// 3. 注册
stack.Defs.Register(&Firework{
	Common: item.Common{ID: "firework", Name: "烟花", MaxStack: 99},
	Burst:  5 * time.Second, Banner: stack.Banner,
})
```

完毕——没有配置表、没有注册工厂、没有中央效果登记。

## 核心概念

| 概念 | 说明 |
|---|---|
| `item.Def` | 道具定义基础接口（标识 + 展示标签 + 堆叠/绑定），`item.Common` 内嵌自动满足 |
| 可选能力接口 | `Usable` / `Equippable` / `EquipGated` / `Expirable` / `Passive` / `RelationGated`，通用层断言分发 |
| `item.Instance` | 玩家持有的实例：数量、绑定、状态、过期时间、乐观锁版本号 |
| `item.Inventory` | 通用库存服务：发放/使用/穿戴/过期/关系流程 |
| 槽位 | avatar / nameplate / badge / vip / mount / headwear / relation / cp_ring / chat_bubble |
| 场景快照 | `item.Snapshot`：按槽位聚合已穿戴道具 + 合并修饰符，按场景 SortPolicy 排序 |
| 关系 | `relation.Relation`：双方主体、类型、可选时效；过期/解除时联动卸下 |

## 目录结构

```
event/            事件接口与发布端口
relation/         双主体关系模型
item/             道具内核：Def/能力接口/Inventory/穿戴/过期/关系流程
item/catalog/     道具类型示例（座驾/勋章/头饰/VIP/关系卡/飘屏/气泡/铭牌/CP戒指/头像框/改名卡/药水/宝箱）
memory/           内存实现：定义注册表、仓储、事件总线、账本、完整 Stack
docs/DESIGN.md    设计文档（v2：显式类型，含与 v1 的差异对照）
```

## 验证

```sh
gofmt -w . && go vet ./... && go test -race ./...
```

Race 模式必需：仓储与事件总线会被并发访问。

## 设计文档

架构决策与 v1 → v2 的推翻重做对照见 [docs/DESIGN.md](docs/DESIGN.md)。

## 已知边界

- 存储仅有内存实现；MySQL / Redis 按核心端口接口接入
- 对方同意流程（关系建立需双方确认）、VIP 每日定时特权未纳入
- 道具定义不支持运行时热更新（有意的设计边界：道具种类全部编译进代码）
