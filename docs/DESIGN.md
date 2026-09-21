# 道具子系统设计（v2：显式类型）

本文档描述 item-system v2 的设计。v1（tag `v1.0.0`，见 git 历史）是一套
"行为组件配置表 + 效果命令物化"的数据驱动架构；v2 按明确的产品判断将其推翻：
**道具系统不需要运行时配置驱动，不需要"不停机上新道具"。所有道具种类都编译进代码，
所以最清晰的表达就是面向对象多态——每种道具一个 Go 类型，行为写在类型的方法里。**

## 1. 核心模型

```go
// 每种道具 = 一个显式类型。行为是方法，标签数据内嵌 item.Common。
type RenameCard struct {
    item.Common                  // ID/Name/Priority/Rarity/MaxStack/Bind
    Users  RenameService         // 道具自己持有外部依赖，构造方注入
}

func (c *RenameCard) Use(ctx context.Context, inv *item.Inventory, owner string, params map[string]any) error {
    newName, _ := params["new_name"].(string)
    if newName == "" { return item.ErrMissingParam }   // 副作用前校验
    return c.Users.Rename(ctx, owner, newName)         // 业务逻辑直接调用外部服务
}

type Mount struct {
    item.Common
    Speed    int
    MinLevel int64
    Levels   LevelSource             // CanEquip 自己查等级，通用层不代劳
    Duration time.Duration           // 0 = 永久；限时款只是不同的构造值
}
```

- **同款不同配置 = 不同构造值**：`&Mount{Speed: 80}`（云朵）vs `&Mount{Speed: 150}`（龙车）。
- **新道具 = 新类型 + 实现想要的能力接口 + 注册一行**。不需要的能力就不实现，
  通用层 `def.(Usable)` 断言自然跳过。

## 2. 分层

```
event/       事件接口 + Publisher（item 与 relation 共享的词汇）
relation/    双主体关系：模型 + Repo 接口 + 关系事件（纯模型，无服务）
item/
  def.go         Def 基础接口 + Common 公共数据 + 可选能力接口
  model.go       实例/槽位/状态/修饰符/穿戴记录/过期策略常量
  events.go      五个领域事件（granted/consumed/equipped/unequipped/expired）
  inventory.go   端口（DefSource/InstanceRepo/EquipRepo/IdempotencyStore）
                 + Inventory：Grant / Use
  equip.go       Equip / Unequip / Build 快照 + 场景排序（SortRegistry/TopN）
  expiry.go      RunExpiry：remove / unequip / downgrade（降级继承目标时效）
  relations.go   BindRelation / DissolveRelation / RunRelationExpiry / BindCP + 解除联动
  catalog/       具体道具类型（座驾/勋章/头饰/VIP/关系卡/飘屏/气泡/铭牌/
                 CP戒指/头像框/改名卡/药水/宝箱）
memory/      全部端口的内存实现 + DefRegistry + 一键装配的 Stack
```

依赖方向单一：`item/catalog → item → relation/event`；`memory → item/relation`；
测试（外部包 `item_test`）→ 以上全部。核心包不 import memory/catalog。

## 3. 可选能力接口（Go 惯例：类型断言分发）

| 接口 | 方法 | 通用层消费点 |
|---|---|---|
| `Def` | DefID/DisplayName/SortPriority/SortRarity/StackLimit/BindOnPickup | Common 内嵌自动满足 |
| `Usable` | `Use(ctx, inv, owner, params)` | Inventory.Use：逐单位调用，成功后扣减 |
| `Equippable` | `EquipSlot()/Capacity()` | Equip：槽位与容量检查 |
| `EquipGated` | `CanEquip(ctx, inv, owner)` | Equip：前置条件由道具自判 |
| `RelationGated` | `RequiresRelation()` | 关系解除/到期联动卸下 |
| `Expirable` | `Lifetime()/ExpirePolicy()/DowngradeTo()` | RunExpiry：三种策略统一执行 |
| `Passive` | `Modifiers()` | Build 快照聚合修饰符 |

## 4. 通用层与道具类型的分工

通用层（Inventory）只做与道具种类无关的事：

- **Grant**：幂等键抢占、堆叠合并（仅同定义+同时效窗口）、时效设置、绑定置位、事件。
- **Use**：归属/状态/数量校验 → 逐单位调用道具 `Use` → 全部成功后 CAS 扣减。
  道具失败即中止（不出现"扣了道具没给效果"），缺参数由道具在副作用前报错。
- **Equip/Unequip**：容量、前置断言、穿戴记录（CAS）。
- **RunExpiry**：remove / unequip / downgrade；降级原地改写实例（同 ID 换定义），
  时效继承目标道具（支持 vip3→vip1→vip0 链）。
- **关系流程**：Bind/Dissolve/RunRelationExpiry + BindCP 组合流程 + 解除联动卸下。
- **Build 快照**：按槽位聚合 + 场景排序（SortRegistry / TopN）。

道具类型负责全部业务行为：改名卡调用户服务、飘屏卡调广播端口、宝箱调 `inv.Grant`
再发放、座驾查等级源自判穿戴条件。外部服务是道具的字段，由装配方构造注入。

## 5. 与 v1 的关键差异（为什么推翻重做）

| v1（数据驱动） | v2（显式类型） |
|---|---|
| 模板 = `Behaviors map[string]map[string]any` 配置 | 模板 = Go 类型 + 类型化字段 |
| Registry + 工厂 + Compile 解码配置 | 无解码；定义即编译产物 |
| 使用产出效果命令，经 Materialize 物化后 Execute | 道具 `Use` 方法直接写业务；params 直传 |
| 每命令私有端口 + 构造注入 + decodeEffect 登记 | 道具字段持有依赖，构造方注入 |
| 7 个领域服务 + application 装配层 + grantAdapter | 单一 Inventory；效果直接调 inv，无环可破 |
| 消费侧窄接口（每服务声明接口切片） | 端口接口声明一次（DefSource/InstanceRepo/...） |
| 新效果 = Command + 构造器 + decodeEffect case + Ports 字段 + Deps 字段（5 处） | 新道具 = 1 个类型 + 方法（1 处） |

**保留不变的通用语义**：模板/实例分离、CAS 乐观锁、幂等键、堆叠合并规则、
使用成功才扣减、三种过期策略、降级继承目标时效、关系解除联动、场景排序、领域事件。

## 6. 扩展指引

- **新道具**：`type X struct { item.Common; ...依赖 }`，实现想要的能力接口，
  `stack.Defs.Register(&X{...})`。完毕。
- **新能力接口**（如"可升级"）：在 item 声明可选接口 + 在相关流程（Inventory 方法）
  增加一个断言分支。
- **真实存储**：按 `item.InstanceRepo / item.EquipRepo / item.DefSource /
  item.IdempotencyStore / relation.Repo` 提供 MySQL/Redis 实现，替换 Stack 中的内存组件。

## 7. 测试约定

- 测试在 `item/` 下的外部包 `item_test`：经 memory/catalog 间接引用被测包，避免循环导入。
- 确定性时间：装配后直接替换 `stack.Inv.Now = func() { return *clock }`，推进 `*clock`。
- 关系过期测试需 `stack.Relations.UseClock(...)`（RelationRepo 内部按自身时钟过滤）。
- `forceExpire` 直接改写实例 ExpireAt（版本 CAS），代替睡眠等待。
- 固定随机：宝箱等道具的 `Rand` 字段注入 `fixedRand`。

## 8. 已知边界

- 存储仅有内存实现；MySQL/Redis 按第 6 节接口接入。
- 对方同意流程（关系建立需双方确认）、VIP 每日定时特权未纳入。
- DefRegistry 注册后无卸载；热更新道具定义需重启进程（这是有意的设计边界）。
