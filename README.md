# item-system

Go 实现的道具子系统（`github.com/Linuxea/item-system`）。发放、使用、穿戴、时效、双主体关系与展示快照。

**一句话说清它的设计：一个道具「有什么能力」，由它「实现了哪些接口」决定。**

```go
type RenameCard struct {
    NewName string `json:"new_name"`  // 参数：玩家使用时传入
    users   port.UserService          // 依赖：构造时注入
}

func (c *RenameCard) Use(ctx context.Context, owner string) error {
    return c.users.Rename(ctx, owner, c.NewName)
}
func (c *RenameCard) ConsumePerUse() int64 { return 1 }
func (c *RenameCard) MaxStack() int64      { return 99 }
```

它实现了 `Use` + `ConsumePerUse`，所以**可使用**；实现了 `MaxStack`，所以**可堆叠**；
没实现 `Slot()`，所以在编译期就**不是可穿戴道具**。没有任何配置文件参与这个判断。

引擎那边只有一行：

```go
u, ok := it.(item.Usable)   // 问：你能用吗？
```

## 三分钟上手

```go
package main

import (
    "context"
    "fmt"

    "github.com/Linuxea/item-system/engine"
    "github.com/Linuxea/item-system/memory"
)

func main() {
    ctx := context.Background()
    s := memory.NewStack(memory.StackOptions{})   // 一行装配全套
    s.Levels.Set("u1", 30)

    // 发一辆座驾并穿上
    res, _ := s.Engine.Grant(ctx, engine.GrantRequest{
        Owner: "u1", ItemID: "mount_dragon", Count: 1, Source: "gm",
    })
    _ = s.Engine.Equip(ctx, "u1", res.InstanceID)

    // 用一张改名卡，参数进的是改名卡自己的字段
    card, _ := s.Engine.Grant(ctx, engine.GrantRequest{
        Owner: "u1", ItemID: "rename_card", Count: 1,
    })
    _, _ = s.Engine.Use(ctx, engine.UseRequest{
        Owner: "u1", InstanceID: card.InstanceID,
        Params: []byte(`{"new_name":"林夕"}`),
    })

    snap, _ := s.Engine.BuildProfile(ctx, "u1", "chat")
    fmt.Println(snap.Modifiers)   // map[move_speed:150]
}
```

完整演示：`go run ./example`

## 八种能力

道具实现哪个接口，就有哪项能力。全部可选，随意组合。

| 接口 | 方法 | 含义 |
|---|---|---|
| `Usable` | `Use` / `ConsumePerUse` | 可主动使用，一次消耗几个 |
| `Parameterized` | `Bind` | 使用时需要玩家输入参数 |
| `Stackable` | `MaxStack` | 可堆叠，单格上限 |
| `Equippable` | `Slot` / `Capacity` | 可穿戴，槽位与容量 |
| `Conditional` | `CanEquip` | 穿戴前置条件，道具自己判断 |
| `Passive` | `Modifiers` | 穿上即生效的被动属性 |
| `Expirable` | `Duration` / `OnExpire` | 有时效，到期怎么处置 |
| `Downgradable` | `DowngradeTo` | 到期降级成哪一款 |
| `Bindable` | `BindOnGrant` | 发放即绑定，不可交易 |
| `Displayable` | `Priority` / `Rarity` | 参与展示排序 |

`items/capabilities.go` 是一张「哪个道具有哪些能力」的对照表，由**编译器**检查。

## 内置道具

| 道具 | 能力组合 |
|---|---|
| 座驾 | 穿戴 + 被动 + 等级门槛 + 展示 + 可选时效 |
| 头饰 / 铭牌 / 头像框 | 穿戴 + 展示 + 可选时效 |
| 勋章 | 穿戴（槽位容量 3）+ 展示 |
| 聊天气泡 | 穿戴 + 被动 + 展示 |
| VIP | 穿戴 + 被动 + 时效 + **降级链** |
| 关系卡 | 穿戴 + 被动 + **关系门槛** + 时效 |
| CP 戒指 | 穿戴 + 被动 + 关系门槛 + **发放即绑定** |
| 改名卡 | 使用 + **参数** + 堆叠 |
| 飘屏卡 | 使用 + 参数 + 堆叠 + 展示 |
| 货币包 | 使用 + 堆叠 |
| 随机宝箱 | 使用 + 堆叠 + 展示（**用了之后再发别的道具**） |

## 目录

```
item/       核心词汇：Item 接口、8 个能力接口、Instance 运行时状态、错误
port/       外部依赖端口：用户服务 / 账本 / 等级 / 广播 / 发放器 / 关系查询
engine/     通用引擎：发放 · 使用 · 穿戴 · 过期 · 展示快照
items/      具体道具实现 + 可复用的能力片段 + 道具册
relation/   双主体关系（独立聚合根）
event/      领域事件
memory/     全部端口的内存实现 + 一键装配 NewStack
example/    可运行的端到端演示
```

依赖方向是单向的：`items → item + port`，`engine → item + event`，
`memory` 在最外层负责把它们接起来。`item` 和 `port` 谁都不依赖。

## 怎么加东西

### 加一款新道具（最常见）

在 `items/` 写一个 struct，嵌入需要的能力片段，登记进 `Catalog`。**引擎一行都不用改。**

```go
type SpeakerCard struct {
    identity
    stack

    Text string `json:"text"`   // 参数
    channel port.Broadcaster    // 依赖
}

func (c *SpeakerCard) ConsumePerUse() int64 { return 1 }
func (c *SpeakerCard) Use(ctx context.Context, owner string) error { ... }
func (c *SpeakerCard) Bind(raw []byte) (item.Usable, error) { ... }
```

### 加一项新能力（少见）

比如「可赠送」：

1. `item/item.go` 加接口 `Giftable { CanGift() bool }`
2. `engine/` 加一个 `Gift` 方法，里面一行 `g, ok := it.(item.Giftable)`
3. 需要这项能力的道具实现它，其余道具**完全不受影响**

这一步需要改代码，这是没法省掉的 —— 新能力意味着新行为，行为总得写在某个地方。
架构能做的是让它**局部**：改动只落在两个文件里，不会散到每个道具上。

### 接真实存储

实现 `engine.InstanceStore` 与 `relation.Repo` 即可，`memory` 包是现成的对照实现：

- `Update(inst, expected)` → `UPDATE ... WHERE id = ? AND version = ?`，影响行数 0 即冲突
- `ListExpiredBefore(t)` → 按到期时间建索引扫描，或用 Redis ZSet 调度

## 值得一提的两个细节

**使用是「先扣减，后执行」。** 顺序是刻意的：CAS 预扣就是并发闸门。
若先执行后扣减，两个并发请求会同时通过数量校验、各自执行一遍效果，之后才有一个扣减失败
—— 道具被用了两次，只扣了一次。执行失败时引擎会把没跑成的份额补偿回去。
`TestConcurrentUseDeductsExactlyOnce` 用 8 个并发请求抢一张卡验证了这一点。

**参数只出现在道具自己的结构体里。** 注册表存的是「原型」（只带依赖，参数字段为零值），
`Bind` 复制出一份副本填上玩家输入，原型永不被修改。
校验写在 `Bind` 里，此刻尚未产生任何副作用 —— 参数不合法时玩家不会白损失一张卡。

## 验证

```sh
gofmt -w . && go vet ./... && go test -race ./...
```

Race 模式必需：仓储与事件总线会被并发访问。

## 已知边界

- 存储只有内存实现
- 建立关系的「对方同意」流程不在本库范围内，`relation.Bind` 只负责双方谈妥后的落地
- 过期靠 `RunExpiry` 主动扫描；真实项目挂定时任务，或用 Redis ZSet 按到期时刻精确调度
