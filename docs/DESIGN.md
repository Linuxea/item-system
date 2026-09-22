# 设计说明

这份文档解释**为什么这么写**。想快速上手看 [README](../README.md)，想看代码从 `item/item.go` 开始。

## 1. 核心主张

> 一个道具「有什么能力」，由它「实现了哪些接口」决定。

这句话有两个对立面，先说清楚不选它们的原因。

**对立面一：继承树。**

```
Item
 ├── EquippableItem
 │    ├── Mount
 │    └── Badge
 └── ConsumableItem
      └── BannerCard
```

问题在于一旦出现「CP 戒指既可穿戴、又要绑定、又有关系前置」这种组合，树就打结了：
要不要造一个 `BindableConditionalEquippableItem`？能力有 8 项，组合有 2^8 种，
继承树最多只能表达其中一条路径。

**对立面二：配置驱动。**

```go
Behaviors: map[string]map[string]any{
    "equippable": {"slot": "mount", "capacity": 1},
    "passive":    {"modifiers": [...]},
}
```

这是游戏行业的标准做法，它换来一个很值钱的能力：**运营后台不发版就能上新道具**。
但代价也实在——`map[string]any` 没有类型安全（`"equipable"` 拼错编译器不吭声，
运行时才炸）、IDE 跳不动（想看座驾怎么工作，得在配置、注册表、组件、服务之间来回翻）、
调试链路变长。

**本库不需要那个能力**，所以不付那笔代价。用接口表达能力，编译期确定，
换来类型安全、可跳转、可重构。代价是上新道具要发版——已知且接受。

## 2. 引擎与道具的分工

引擎负责**所有道具共有的那部分**：

- 实例的数量、绑定、穿戴状态
- 乐观锁（CAS）与幂等
- 堆叠合并
- 时效计算与到期扫描
- 槽位容量
- 展示快照的聚合与排序
- 领域事件

道具负责**只有它自己知道的那部分**：

- 「用了会发生什么」（`Use`）
- 「参数长什么样、怎么校验」（`Bind`）
- 「能不能穿」（`CanEquip`）
- 「给什么属性」（`Modifiers`）

两者的接缝就是 `item` 包里那 8 个接口。引擎在需要分支的地方问一句：

```go
st, ok := it.(item.Stackable)     // 发放时：能堆叠吗？
u,  ok := it.(item.Usable)        // 使用时：能用吗？
eq, ok := it.(item.Equippable)    // 穿戴时：能穿吗？穿哪儿？
c,  ok := it.(item.Conditional)   // 穿戴时：有前置条件吗？
p,  ok := it.(item.Passive)       // 快照时：有被动属性吗？
ex, ok := it.(item.Expirable)     // 过期时：怎么处置？
d,  ok := it.(item.Downgradable)  // 降级时：降成哪款？
b,  ok := it.(item.Bindable)      // 发放时：要绑定吗？
```

全部的分支就这 8 处。`engine` 包里找不到任何一个具体道具的名字。

## 3. 参数为什么放在道具结构体里

需求：飘屏卡的文案、改名卡的新昵称，要在**使用时**由玩家输入。

一个常见的做法是让参数穿过接口：

```go
Use(ctx, owner string, params map[string]any) error
```

不这么做的原因是：`params` 是个什么都能塞的口袋。每个道具都得自己从里面掏、
自己判类型、自己处理缺失，而且**编译器帮不上任何忙**。加一个参数不改签名，
少传一个参数也不报错。

本库的做法是**参数就是道具结构体的公开字段**：

```go
type BannerCard struct {
    Text string `json:"text"`   // 参数

    seconds     int             // 配置（构造时定死）
    broadcaster port.Broadcaster // 依赖（构造时注入）
}
```

注册表里存的是**原型**：`Text` 为空，只带依赖。使用时引擎调 `Bind`：

```go
func (c *BannerCard) Bind(raw []byte) (item.Usable, error) {
    bound := *c                        // 复制原型，依赖一并带过来
    if err := bindJSON(raw, &bound); err != nil {
        return nil, err
    }
    if strings.TrimSpace(bound.Text) == "" {
        return nil, fmt.Errorf("%w: text", item.ErrMissingParam)
    }
    return &bound, nil
}
```

得到一个填好参数的副本，原型分毫不动——多个玩家同时用飘屏卡，各拿各的副本，天然并发安全。

`Use` 的签名里因此没有任何参数容器：

```go
func (c *BannerCard) Use(ctx context.Context, owner string) error {
    return c.broadcaster.Broadcast(ctx, owner, c.Text, c.seconds)
}
```

**校验写在 `Bind` 里，而 `Bind` 发生在任何副作用之前。** 参数不合法时引擎还没扣任何东西，
玩家不会白损失一张卡。这个不变量靠的是调用顺序，不需要任何中央机制来保证。

唯一没能消掉的动态成分是 `Bind` 的入参 `[]byte`——它是 API 边界传进来的原始 JSON。
但解码发生在道具**自己的文件里**，解进的是**自己的字段**，这和「一个全局 params map
在整个系统里穿来穿去」是两回事。

## 4. 使用为什么「先扣减，后执行」

`engine/use.go` 的顺序是：

```
校验与参数绑定   ← 零副作用
CAS 预扣        ← 并发闸门
执行道具逻辑     ← 真正的副作用
失败则补偿回滚
```

反过来（先执行后扣减）有一个真实的并发漏洞：两个请求同时读到 `Count = 1`，
都通过数量校验，都执行了一遍效果，然后其中一个扣减时才发现版本冲突。
**道具被用了两次，只扣了一次。**

把 CAS 扣减提到执行之前，它就成了闸门：只有一个请求能扣成功，另一个直接拿到版本冲突，
根本走不到执行那一步。代价是执行失败时要补偿回滚，这段逻辑写在同一个函数里，
回滚失败会如实上报（那是需要人工介入的状态，不该被吞掉）。

`memory/grant_use_test.go` 里的 `TestConcurrentUseDeductsExactlyOnce` 用 8 个
goroutine 抢一张改名卡，断言恰好 1 成功 7 失败。

## 5. 三个环是怎么解开的

装配时有三处循环依赖，都用「先造空壳、后回注」解决：

**环一：宝箱 → 发放器 → 引擎 → 宝箱**

宝箱要发奖品，发奖品就是引擎的 `Grant`，而引擎要先有全部道具才能构造。
`engine.LateGranter` 先造一个空壳交给宝箱，引擎构造完再 `Bind(eng)`。

**环二：关系卡 → 关系服务 → 引擎 → 关系卡**

关系解除时要联动卸下道具，那是引擎的 `UnequipSlot`。
`relation.Service.BindUnequipper(eng)` 同理。

**环三（不存在的那个）：引擎 → 关系**

引擎完全不知道「关系」是什么。CP 戒指自己嵌入 `relationGate` 片段、
自己去问 `port.RelationChecker`，引擎照旧只调一个 `CanEquip`。
这个环根本没形成，因为职责切对了。

装配的完整顺序见 `memory/stack.go` 的 `NewStack`——那是全程序唯一一处「全量依赖」
出现的地方。之后运行期的每次调用都只带 `(ctx, owner)`。

## 6. 能力片段：Go 的组合

`items/fragments.go` 里有几个可嵌入的小结构体：

```go
type wearable struct {
    slot     item.Slot
    capacity int
}
func (w wearable) Slot() item.Slot { return w.slot }
func (w wearable) Capacity() int   { return w.capacity }
```

道具嵌入它就自动实现了 `item.Equippable`，不用每个道具重写一遍访问器。

```go
type Mount struct {
    identity
    display
    wearable
    levelGate
    timed
    speed int64
}
```

一眼看出座驾有哪些能力。这就是配置表 `Behaviors: {...}` 的编译期版本——
表达力相同，但拼错名字编译不过。

`timed` 有个顺手的性质：`duration` 为 0 时引擎不给实例设到期时间，等同于永久。
所以「有的座驾限时、有的永久」不需要两个类型，构造时传不同的 `duration` 即可。

## 7. 一条没能省掉的规律

新增**道具**是免费的：写个 struct，登记进 `Catalog`，引擎一行不动。

新增**能力**不是免费的：要在 `item` 加接口，还要在 `engine` 加一处断言。
没有任何架构能省掉后半句——新能力意味着新行为，行为总得写在某个地方。
ECS 也一样：加一个 Component 是免费的，但你必须写一个 System 去读它，
否则它就是一堆躺在内存里没人理的字节。

架构能做的不是让「新增能力」变免费，而是让它**局部**：
改动落在两个文件里，不会散到每一个道具上。这是本库对那条规律的全部回应。
