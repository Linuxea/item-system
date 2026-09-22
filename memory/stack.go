package memory

import (
	"time"

	"github.com/Linuxea/item-system/engine"
	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/items"
	"github.com/Linuxea/item-system/port"
	"github.com/Linuxea/item-system/relation"
)

// Stack 一套装配完成、可直接使用的道具体系。
type Stack struct {
	// Engine 道具引擎。
	Engine *engine.Engine
	// Relations 关系服务。
	Relations *relation.Service
	// Registry 道具注册表。
	Registry *item.Registry

	// 下面是各端口的内存实现，测试与演示里直接读它们断言结果。
	Instances   *InstanceRepo
	RelationRep *RelationRepo
	Idempotency *Idempotency
	Bus         *EventBus
	Users       *Users
	Ledger      *Ledger
	Levels      *Levels
	Broadcaster *Broadcaster
	Clock       *Clock
}

// StackOptions 装配参数，全部可省略。
type StackOptions struct {
	// Now 起始时刻；零值时用 time.Now()。
	Now time.Time
	// Rand 随机数来源；为 nil 时用一个简单的确定性实现（总是取第一档）。
	Rand port.Rand
	// Scenes 各展示场景的排序策略。
	Scenes map[string]engine.SortPolicy
	// ExtraItems 额外的自定义道具，会与内置道具册合并。
	ExtraItems []item.Item
}

// NewStack 装配一整套内存实现的道具体系。
//
// 这里是整个程序里唯一的「装配点」：全量依赖只在这个函数里出现一次，
// 每个道具从中取走自己需要的那一两个端口，之后运行期不再有依赖穿越调用。
//
// 装配顺序要解开一个环：
//
//	道具需要 关系服务 与 发放器
//	发放器   就是 引擎
//	引擎     需要 全部道具
//
// 做法是先把关系服务和发放器造成空壳交给道具，引擎构造完成后再回注（Bind）。
func NewStack(opts StackOptions) *Stack {
	start := opts.Now
	if start.IsZero() {
		start = time.Now()
	}
	clock := NewClock(start)

	rnd := opts.Rand
	if rnd == nil {
		rnd = func(int64) int64 { return 0 }
	}

	instances := NewInstanceRepo()
	relations := NewRelationRepo()
	idem := NewIdempotency()
	bus := NewEventBus()
	users := NewUsers()
	ledger := NewLedger()
	levels := NewLevels()
	broadcaster := NewBroadcaster()

	// 1. 关系服务先造出来（此时还没有引擎，联动卸下器稍后回注）。
	relSvc := relation.New(relation.Options{
		Repo:      relations,
		Publisher: bus,
		Now:       clock.Now,
	})

	// 2. 发放器造成空壳（引擎稍后回注）。
	granter := engine.NewLateGranter("chest", "open")

	// 3. 道具册：每个道具在这里拿走自己需要的端口。
	catalog := items.Catalog(items.Deps{
		Users:       users,
		Ledger:      ledger,
		Levels:      levels,
		Broadcaster: broadcaster,
		Granter:     granter,
		Rand:        rnd,
		Relations:   relSvc,
	})
	catalog = append(catalog, opts.ExtraItems...)
	registry := item.NewRegistry(catalog...)

	// 4. 引擎。
	eng := engine.New(engine.Options{
		Registry:    registry,
		Store:       instances,
		Idempotency: idem,
		Publisher:   bus,
		Now:         clock.Now,
		Scenes:      opts.Scenes,
	})

	// 5. 回注，环解开。
	granter.Bind(eng)
	relSvc.BindUnequipper(eng)

	return &Stack{
		Engine:      eng,
		Relations:   relSvc,
		Registry:    registry,
		Instances:   instances,
		RelationRep: relations,
		Idempotency: idem,
		Bus:         bus,
		Users:       users,
		Ledger:      ledger,
		Levels:      levels,
		Broadcaster: broadcaster,
		Clock:       clock,
	}
}
