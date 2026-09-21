// Package behavior 提供道具的行为组件体系与模板编译器。
// 道具 = 行为组件的组合：模板 Behaviors 配置经 Registry 编译为 Compiled（组件集合），
// 新道具类型只需新增配置数据，不需要改动任何核心流程（组合优于继承）。
package behavior

import (
	"context"
	"fmt"
	"time"

	"github.com/Linuxea/item-system/domain/effect"
	"github.com/Linuxea/item-system/domain/model"
)

// 行为组件 key：模板 Behaviors 配置 map 的键。
const (
	// KeyStackable 可堆叠（max_stack）。
	KeyStackable = "stackable"
	// KeyExpirable 可过期（duration/on_expire/downgrade_to）。
	KeyExpirable = "expirable"
	// KeyEquippable 可穿戴（slot/capacity）。
	KeyEquippable = "equippable"
	// KeyUsable 可主动使用（effects 效果列表）。
	KeyUsable = "usable"
	// KeyBindable 可绑定（bind_on_pickup）。
	KeyBindable = "bindable"
	// KeyPassive 被动修饰（modifiers）。
	KeyPassive = "passive"
	// KeyCondition 穿戴前置条件（min_level/requires_relation）。
	KeyCondition = "condition"
)

// 过期策略常量。
const (
	// ExpirePolicyRemove 过期即删除实例（含穿戴记录）。
	ExpirePolicyRemove = "remove"
	// ExpirePolicyUnequip 过期仅卸下并保留实例。
	ExpirePolicyUnequip = "unequip"
	// ExpirePolicyDowngrade 过期降级为 downgrade_to 指定的模板。
	ExpirePolicyDowngrade = "downgrade"
)

// Behavior 行为组件标记接口，Key 返回组件 key。
type Behavior interface {
	Key() string
}

// Stackable 可堆叠组件：同模板、同时效的实例可合并，MaxStack 为单实例数量上限；
// MaxStack <= 1 视为不可堆叠。
type Stackable struct {
	MaxStack int64
}

func (b Stackable) Key() string { return KeyStackable }

// Expirable 可过期组件：Duration 为有效期；OnExpire 为过期策略；
// DowngradeTo 仅在 downgrade 策略下生效（降级目标继承目标模板自身的时效）。
type Expirable struct {
	Duration    time.Duration
	OnExpire    string
	DowngradeTo string
}

func (b Expirable) Key() string { return KeyExpirable }

// Equippable 可穿戴组件：Slot 为目标槽位，Capacity 为该槽位对此道具的容量上限。
type Equippable struct {
	Slot     model.SlotType
	Capacity int
}

func (b Equippable) Key() string { return KeyEquippable }

// Usable 可使用组件：Effects 为使用时产出的效果命令（静态配置部分），
// 用户动态输入（如飘屏文案）由 effect.Materialize 在使用阶段物化进命令。
type Usable struct {
	Effects []effect.Command
}

func (b Usable) Key() string { return KeyUsable }

// Bindable 绑定组件：BindOnPickup 为 true 时发放即绑定，实例不可交易。
type Bindable struct {
	BindOnPickup bool
}

func (b Bindable) Key() string { return KeyBindable }

// Passive 被动修饰组件：穿戴后生效的修饰符集合，由 profile 快照聚合。
type Passive struct {
	Modifiers []model.Modifier
}

func (b Passive) Key() string { return KeyPassive }

// Condition 穿戴前置条件组件。
// 条件由组件自己判断（Met），EquipService 只调用不代劳；
// checker 为私有端口，由装配点注入，领域内不依赖任何具体校验实现。
type Condition struct {
	MinLevel         int64
	RequiresRelation string
	checker          ConditionChecker
}

func (b Condition) Key() string { return KeyCondition }

// Met 判断玩家是否满足条件；未注入 checker 时视为无条件放行（纯数据场景）。
func (b Condition) Met(ctx context.Context, owner string) (bool, error) {
	if b.checker == nil {
		return true, nil
	}
	return b.checker.Satisfied(ctx, owner, b)
}

// ConditionChecker 条件校验端口：实现方综合玩家等级、关系状态等外部数据做判断。
type ConditionChecker interface {
	Satisfied(ctx context.Context, owner string, cond Condition) (bool, error)
}

// Factory 组件工厂：把模板里的原始配置 map 编译为组件实例。
type Factory func(cfg map[string]any) (Behavior, error)

// Ports 启动装配期的端口集合，只在 NewRegistry 时使用一次，绝不进入任何运行期调用。
// 每个端口服务于特定效果命令（Ledger→AddCurrency，Banner→BroadcastBanner 等）。
type Ports struct {
	Ledger     effect.Ledger
	Banner     effect.BannerBroadcaster
	Granter    effect.Granter
	Rand       func(n int64) int64
	Conditions ConditionChecker
}

// Registry 行为注册表：key -> 组件工厂。
// 同时实现 Compiler 窄接口，领域服务只见 Compile 一个方法，不见注册能力。
type Registry struct {
	factories map[string]Factory
	ports     Ports
}

// NewRegistry 创建注册表并登记全部内置行为组件。
// usable/condition 的工厂闭包持有 ports，使效果命令在编译期即绑定自己的端口。
func NewRegistry(ports Ports) *Registry {
	r := &Registry{factories: map[string]Factory{}, ports: ports}
	r.Register(KeyStackable, stackableFactory)
	r.Register(KeyExpirable, expirableFactory)
	r.Register(KeyEquippable, equippableFactory)
	r.Register(KeyUsable, func(cfg map[string]any) (Behavior, error) {
		return usableFactory(cfg, r.ports)
	})
	r.Register(KeyBindable, bindableFactory)
	r.Register(KeyPassive, passiveFactory)
	r.Register(KeyCondition, func(cfg map[string]any) (Behavior, error) {
		return conditionFactory(cfg, r.ports)
	})
	return r
}

// Compiler 领域服务消费的窄接口：只暴露模板编译这一个能力。
type Compiler interface {
	Compile(tpl *model.ItemTemplate) (*Compiled, error)
}

// Register 登记一个行为组件工厂（扩展点）。
func (r *Registry) Register(key string, f Factory) {
	r.factories[key] = f
}

// Compiled 编译产物：模板 + 按需实例化的组件集合。
// 各 typed 访问器（Stackable/Equippable/...）返回组件是否存在（ok），
// 调用方以 ok 分支表达"该道具有/没有此行为"，而非错误。
type Compiled struct {
	Template  *model.ItemTemplate
	behaviors map[string]Behavior
}

// Get 按 key 取组件。
func (c *Compiled) Get(key string) (Behavior, bool) {
	b, ok := c.behaviors[key]
	return b, ok
}

// Stackable 返回可堆叠组件（若配置）。
func (c *Compiled) Stackable() (Stackable, bool) {
	b, ok := c.behaviors[KeyStackable].(Stackable)
	return b, ok
}

// Expirable 返回可过期组件（若配置）。
func (c *Compiled) Expirable() (Expirable, bool) {
	b, ok := c.behaviors[KeyExpirable].(Expirable)
	return b, ok
}

// Equippable 返回可穿戴组件（若配置）。
func (c *Compiled) Equippable() (Equippable, bool) {
	b, ok := c.behaviors[KeyEquippable].(Equippable)
	return b, ok
}

// Usable 返回可使用组件（若配置）。
func (c *Compiled) Usable() (Usable, bool) {
	b, ok := c.behaviors[KeyUsable].(Usable)
	return b, ok
}

// Bindable 返回绑定组件（若配置）。
func (c *Compiled) Bindable() (Bindable, bool) {
	b, ok := c.behaviors[KeyBindable].(Bindable)
	return b, ok
}

// Passive 返回被动修饰组件（若配置）。
func (c *Compiled) Passive() (Passive, bool) {
	b, ok := c.behaviors[KeyPassive].(Passive)
	return b, ok
}

// Condition 返回前置条件组件（若配置）。
func (c *Compiled) Condition() (Condition, bool) {
	b, ok := c.behaviors[KeyCondition].(Condition)
	return b, ok
}

// Compile 把模板的 Behaviors 配置编译为组件集合；未登记的 key 或非法配置直接报错。
func (r *Registry) Compile(tpl *model.ItemTemplate) (*Compiled, error) {
	c := &Compiled{Template: tpl, behaviors: map[string]Behavior{}}
	for key, cfg := range tpl.Behaviors {
		f, ok := r.factories[key]
		if !ok {
			return nil, fmt.Errorf("behavior %q of template %q: %w", key, tpl.ID, ErrUnknownBehavior)
		}
		b, err := f(cfg)
		if err != nil {
			return nil, fmt.Errorf("behavior %q of template %q: %w", key, tpl.ID, err)
		}
		c.behaviors[key] = b
	}
	return c, nil
}

// ErrUnknownBehavior 模板引用了未登记的行为 key。
var ErrUnknownBehavior = fmt.Errorf("unknown behavior")

func stackableFactory(cfg map[string]any) (Behavior, error) {
	return Stackable{MaxStack: getInt(cfg, "max_stack", 1)}, nil
}

func expirableFactory(cfg map[string]any) (Behavior, error) {
	d, err := getDuration(cfg, "duration")
	if err != nil {
		return nil, err
	}
	// 默认过期策略为 remove；downgrade 需显式给出 downgrade_to 才完整。
	policy := getString(cfg, "on_expire", ExpirePolicyRemove)
	switch policy {
	case ExpirePolicyRemove, ExpirePolicyUnequip, ExpirePolicyDowngrade:
	default:
		return nil, fmt.Errorf("invalid on_expire policy %q", policy)
	}
	return Expirable{
		Duration:    d,
		OnExpire:    policy,
		DowngradeTo: getString(cfg, "downgrade_to", ""),
	}, nil
}

func equippableFactory(cfg map[string]any) (Behavior, error) {
	slot := getString(cfg, "slot", "")
	if slot == "" {
		return nil, fmt.Errorf("equippable requires slot")
	}
	return Equippable{
		Slot:     model.SlotType(slot),
		Capacity: int(getInt(cfg, "capacity", 1)),
	}, nil
}

// usableFactory 编译效果列表：每项配置经 decodeEffect 变为命令实例（端口在此绑定）。
func usableFactory(cfg map[string]any, ports Ports) (Behavior, error) {
	raw, ok := cfg["effects"].([]any)
	if !ok {
		return nil, fmt.Errorf("usable requires effects list")
	}
	cmds := make([]effect.Command, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("effect entry must be a map")
		}
		cmd, err := decodeEffect(m, ports)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return Usable{Effects: cmds}, nil
}

// decodeEffect 效果配置 -> 命令实例的中央映射。
// 新增效果 = 新增 Command struct + 此处一个 case（必要时 Ports 加一个端口字段），
// 这是"无中央 type-switch 执行器"原则下唯一的集中登记点。
func decodeEffect(m map[string]any, ports Ports) (effect.Command, error) {
	kind := getString(m, "kind", "")
	switch kind {
	case effect.KindAddCurrency:
		return effect.NewAddCurrency(
			ports.Ledger,
			getString(m, "currency", ""),
			getInt(m, "amount", 0),
		), nil
	case effect.KindGrantItem:
		return effect.NewGrantItem(
			ports.Granter,
			getString(m, "template_id", ""),
			getInt(m, "count", 1),
		), nil
	case effect.KindRandomGrant:
		raw, ok := m["entries"].([]any)
		if !ok {
			return nil, fmt.Errorf("random_grant requires entries")
		}
		entries := make([]effect.RandomEntry, 0, len(raw))
		for _, e := range raw {
			em, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("random entry must be a map")
			}
			entries = append(entries, effect.RandomEntry{
				TemplateID: getString(em, "template_id", ""),
				Count:      getInt(em, "count", 1),
				Weight:     getInt(em, "weight", 1),
			})
		}
		return effect.NewRandomGrant(ports.Granter, ports.Rand, entries), nil
	case effect.KindBroadcastBanner:
		d, err := getDuration(m, "duration")
		if err != nil {
			return nil, err
		}
		// param 指明用户输入从哪个键取文案（默认 "text"），物化时使用。
		return effect.NewBroadcastBanner(
			ports.Banner,
			d,
			getString(m, "param", "text"),
		), nil
	default:
		return nil, fmt.Errorf("unknown effect kind %q", kind)
	}
}

func bindableFactory(cfg map[string]any) (Behavior, error) {
	return Bindable{BindOnPickup: getBool(cfg, "bind_on_pickup", false)}, nil
}

func passiveFactory(cfg map[string]any) (Behavior, error) {
	raw, ok := cfg["modifiers"].([]any)
	if !ok {
		return nil, fmt.Errorf("passive requires modifiers list")
	}
	mods := make([]model.Modifier, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("modifier entry must be a map")
		}
		mods = append(mods, model.Modifier{
			Key:   getString(m, "key", ""),
			Value: m["value"],
		})
	}
	return Passive{Modifiers: mods}, nil
}

func conditionFactory(cfg map[string]any, ports Ports) (Behavior, error) {
	return Condition{
		MinLevel:         getInt(cfg, "min_level", 0),
		RequiresRelation: getString(cfg, "requires_relation", ""),
		checker:          ports.Conditions,
	}, nil
}

// getInt 宽容地读取整型配置：兼容 int64/int/float64（不同来源的反序列化差异），缺失时返回默认值。
func getInt(cfg map[string]any, key string, def int64) int64 {
	if v, ok := cfg[key].(int64); ok {
		return v
	}
	if v, ok := cfg[key].(int); ok {
		return int64(v)
	}
	if v, ok := cfg[key].(float64); ok {
		return int64(v)
	}
	return def
}

// getString 读取字符串配置，缺失时返回默认值。
func getString(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key].(string); ok {
		return v
	}
	return def
}

// getBool 读取布尔配置，缺失时返回默认值。
func getBool(cfg map[string]any, key string, def bool) bool {
	if v, ok := cfg[key].(bool); ok {
		return v
	}
	return def
}

// getDuration 读取时长配置（如 "72h"），缺失或非法即报错。
func getDuration(cfg map[string]any, key string) (time.Duration, error) {
	s := getString(cfg, key, "")
	if s == "" {
		return 0, fmt.Errorf("missing duration")
	}
	return time.ParseDuration(s)
}
