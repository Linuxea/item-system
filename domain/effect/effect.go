// Package effect 定义道具使用时产出的效果命令体系。
// 每个命令自带数据（做什么）与私有端口（拿什么做），构造时注入依赖，Exec 时使用；
// 用户动态输入只在 Materialize（物化缝）绑定，命令出厂即完整，执行缝不再感知参数。
package effect

import (
	"context"
	"errors"
	"time"
)

// 效果命令种类常量，与模板配置里的 kind 字段一一对应。
const (
	// KindAddCurrency 增加货币。
	KindAddCurrency = "add_currency"
	// KindGrantItem 再发放指定道具。
	KindGrantItem = "grant_item"
	// KindRandomGrant 按权重随机发放（宝箱）。
	KindRandomGrant = "random_grant"
	// KindBroadcastBanner 全服飘屏广播。
	KindBroadcastBanner = "broadcast_banner"
)

// Command 效果命令接口：两个方法，且永不增长。
// 新效果 = 新增命令 struct，不改本接口（变化落在数据上，不落在接缝形状上）。
type Command interface {
	Kind() string
	Exec(ctx context.Context, owner string) error
}

// Ledger 货币账本端口：AddCurrency 命令的唯一依赖。
type Ledger interface {
	Add(ctx context.Context, owner, currency string, amount int64) error
}

// BannerBroadcaster 飘屏广播端口：BroadcastBanner 命令的唯一依赖。
type BannerBroadcaster interface {
	Broadcast(ctx context.Context, owner, text string, duration time.Duration) error
}

// Granter 再发放端口：GrantItem/RandomGrant 命令的唯一依赖。
// 由装配点的适配器实现（转发到 grant.Service），避免 effect 反向依赖 grant 包。
type Granter interface {
	GrantEffect(ctx context.Context, owner, templateID string, count int64, reason string) error
}

// AddCurrency 加货币命令：数据（币种、数额）+ 私有端口（账本）。
type AddCurrency struct {
	Currency string
	Amount   int64
	ledger   Ledger
}

// NewAddCurrency 构造命令并注入账本端口。
func NewAddCurrency(ledger Ledger, currency string, amount int64) AddCurrency {
	return AddCurrency{Currency: currency, Amount: amount, ledger: ledger}
}

func (c AddCurrency) Kind() string { return KindAddCurrency }

func (c AddCurrency) Exec(ctx context.Context, owner string) error {
	// 端口未注入（纯数据场景）时静默跳过，保证命令可作为配置安全传递。
	if c.ledger == nil {
		return nil
	}
	return c.ledger.Add(ctx, owner, c.Currency, c.Amount)
}

// GrantItem 再发放命令：使用道具后发放另一个指定道具。
type GrantItem struct {
	TemplateID string
	Count      int64
	granter    Granter
}

// NewGrantItem 构造命令并注入再发放端口。
func NewGrantItem(granter Granter, templateID string, count int64) GrantItem {
	return GrantItem{TemplateID: templateID, Count: count, granter: granter}
}

func (c GrantItem) Kind() string { return KindGrantItem }

func (c GrantItem) Exec(ctx context.Context, owner string) error {
	if c.granter == nil {
		return nil
	}
	return c.granter.GrantEffect(ctx, owner, c.TemplateID, c.Count, KindGrantItem)
}

// RandomEntry 随机发放的一个候选条目，Weight 为抽取权重。
type RandomEntry struct {
	TemplateID string
	Count      int64
	Weight     int64
}

// RandomGrant 随机发放命令（宝箱）：按总权重随机命中一个条目后发放。
type RandomGrant struct {
	Entries []RandomEntry
	granter Granter
	rand    func(n int64) int64
}

// NewRandomGrant 构造命令，注入再发放端口与随机源（可注入固定 RNG 供测试）。
func NewRandomGrant(granter Granter, rand func(n int64) int64, entries []RandomEntry) RandomGrant {
	return RandomGrant{Entries: entries, granter: granter, rand: rand}
}

func (c RandomGrant) Kind() string { return KindRandomGrant }

func (c RandomGrant) Exec(ctx context.Context, owner string) error {
	if c.granter == nil {
		return nil
	}
	entry, ok := c.pick()
	if !ok {
		return nil
	}
	return c.granter.GrantEffect(ctx, owner, entry.TemplateID, entry.Count, KindRandomGrant)
}

// pick 加权随机抽取：在 [0, total) 区间按权重顺序命中条目。
func (c RandomGrant) pick() (RandomEntry, bool) {
	if len(c.Entries) == 0 || c.rand == nil {
		return RandomEntry{}, false
	}
	var total int64
	for _, e := range c.Entries {
		total += e.Weight
	}
	if total <= 0 {
		return RandomEntry{}, false
	}
	n := c.rand(total)
	for _, e := range c.Entries {
		if n < e.Weight {
			return e, true
		}
		n -= e.Weight
	}
	// 理论不可达，兜底返回最后一个条目。
	return c.Entries[len(c.Entries)-1], true
}

// BroadcastBanner 飘屏命令：文案 Text 属于用户动态输入，
// 在物化（WithParams）前为空；paramKey 记录从哪个参数键取文案。
type BroadcastBanner struct {
	Text     string
	Duration time.Duration
	paramKey string
	banner   BannerBroadcaster
}

// NewBroadcastBanner 构造静态部分（时长、参数键、广播端口），文案待物化绑定。
func NewBroadcastBanner(banner BannerBroadcaster, duration time.Duration, paramKey string) BroadcastBanner {
	return BroadcastBanner{Duration: duration, paramKey: paramKey, banner: banner}
}

func (c BroadcastBanner) Kind() string { return KindBroadcastBanner }

func (c BroadcastBanner) Exec(ctx context.Context, owner string) error {
	if c.banner == nil {
		return nil
	}
	return c.banner.Broadcast(ctx, owner, c.Text, c.Duration)
}

// Parametrized 动态命令标记接口：需要用户参数补全的命令实现 WithParams。
// 是否动态是命令自己的知识，物化缝以类型断言发现，不维护中央清单。
type Parametrized interface {
	WithParams(params map[string]any) (Command, error)
}

// WithParams 用用户输入补全文案：已有文案（模板预置）直接通过；
// 缺参数键或取不到非空字符串则返回 ErrMissingParam——零副作用失败。
func (c BroadcastBanner) WithParams(params map[string]any) (Command, error) {
	if c.Text != "" {
		return c, nil
	}
	if c.paramKey == "" {
		return nil, ErrMissingParam
	}
	text, _ := params[c.paramKey].(string)
	if text == "" {
		return nil, ErrMissingParam
	}
	c.Text = text
	return c, nil
}

// ErrMissingParam 物化时缺失必需的用户参数。
var ErrMissingParam = errors.New("required use param missing")

// Materialize 物化缝：全系统唯一消费用户输入（Params）的地方。
// 静态命令原样通过；动态命令（Parametrized）在此绑定载荷成为完整命令。
// 任何参数缺失都在这里失败，早于一切副作用发生。
func Materialize(cmds []Command, params map[string]any) ([]Command, error) {
	out := make([]Command, len(cmds))
	for i, cmd := range cmds {
		p, ok := cmd.(Parametrized)
		if !ok {
			out[i] = cmd
			continue
		}
		materialized, err := p.WithParams(params)
		if err != nil {
			return nil, err
		}
		out[i] = materialized
	}
	return out, nil
}

// Execute 执行缝：顺序执行"已完整"的命令，接口只依赖这一不变量，
// 因此签名自确立后不再变更；某个命令失败即中止并返回错误。
func Execute(ctx context.Context, owner string, cmds []Command) error {
	for _, cmd := range cmds {
		if err := cmd.Exec(ctx, owner); err != nil {
			return err
		}
	}
	return nil
}
