package items

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Linuxea/item-system/item"
	"github.com/Linuxea/item-system/port"
)

// bindJSON 把玩家输入的原始参数解进 dst。
// raw 为空时不做解析，直接交给随后的字段校验报「缺少必需参数」。
func bindJSON(raw []byte, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("%w: %v", item.ErrInvalidParam, err)
	}
	return nil
}

// ============================================================
// 改名卡
// 能力：可使用 + 需要参数 + 可堆叠
//
// 参数就是结构体的公开字段。注册表里存的原型 NewName 为空、只带依赖；
// Bind 复制出一份副本填上玩家输入，原型始终不被修改（并发安全）。
// ============================================================

// RenameCard 改名卡：使用时由玩家输入新昵称。
type RenameCard struct {
	identity
	stack

	// NewName 新昵称，由玩家在使用时传入。
	NewName string `json:"new_name"`

	users port.UserService // 私有依赖，构造时注入
}

// NewRenameCard 构造改名卡原型。
func NewRenameCard(users port.UserService) *RenameCard {
	return &RenameCard{
		identity: identity{id: "rename_card", name: "改名卡"},
		stack:    stack{max: 99},
		users:    users,
	}
}

// ConsumePerUse 一次改名消耗一张。
func (c *RenameCard) ConsumePerUse() int64 { return 1 }

// Bind 绑定玩家输入并校验。
// 校验失败时返回错误 —— 此刻引擎还没扣任何东西，玩家不会白损失一张卡。
func (c *RenameCard) Bind(raw []byte) (item.Usable, error) {
	bound := *c
	if err := bindJSON(raw, &bound); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(bound.NewName)
	if name == "" {
		return nil, fmt.Errorf("%w: new_name", item.ErrMissingParam)
	}
	if n := len([]rune(name)); n < 2 || n > 16 {
		return nil, fmt.Errorf("%w: 昵称长度需在 2-16 字之间，当前 %d 字", item.ErrInvalidParam, n)
	}
	bound.NewName = name
	return &bound, nil
}

// Use 改名。读的是自己的字段，签名里没有任何参数容器。
func (c *RenameCard) Use(ctx context.Context, owner string) error {
	return c.users.Rename(ctx, owner, c.NewName)
}

// ============================================================
// 全服飘屏卡
// 能力：可使用 + 需要参数 + 可堆叠
// ============================================================

// BannerCard 飘屏卡：使用时由玩家输入要广播的文案。
type BannerCard struct {
	identity
	display
	stack

	// Text 广播文案，由玩家在使用时传入。
	Text string `json:"text"`

	seconds     int
	broadcaster port.Broadcaster
}

// NewBannerCard 构造飘屏卡原型；seconds 为飘屏停留秒数。
func NewBannerCard(id, name string, rarity, seconds int, b port.Broadcaster) *BannerCard {
	return &BannerCard{
		identity:    identity{id: id, name: name},
		display:     display{priority: 50, rarity: rarity},
		stack:       stack{max: 99},
		seconds:     seconds,
		broadcaster: b,
	}
}

// ConsumePerUse 一次飘屏消耗一张。
func (c *BannerCard) ConsumePerUse() int64 { return 1 }

// Bind 绑定文案并校验长度。
func (c *BannerCard) Bind(raw []byte) (item.Usable, error) {
	bound := *c
	if err := bindJSON(raw, &bound); err != nil {
		return nil, err
	}
	text := strings.TrimSpace(bound.Text)
	if text == "" {
		return nil, fmt.Errorf("%w: text", item.ErrMissingParam)
	}
	if n := len([]rune(text)); n > 50 {
		return nil, fmt.Errorf("%w: 飘屏文案不超过 50 字，当前 %d 字", item.ErrInvalidParam, n)
	}
	bound.Text = text
	return &bound, nil
}

// Use 发起全服广播。
func (c *BannerCard) Use(ctx context.Context, owner string) error {
	return c.broadcaster.Broadcast(ctx, owner, c.Text, c.seconds)
}

// ============================================================
// 货币包
// 能力：可使用 + 可堆叠（不需要参数，所以没有 Bind）
// ============================================================

// CurrencyPack 货币包：使用后往账本加一笔货币。
type CurrencyPack struct {
	identity
	stack
	currency string
	amount   int64
	ledger   port.Ledger
}

// NewCurrencyPack 构造货币包。
func NewCurrencyPack(id, name, currency string, amount int64, ledger port.Ledger) *CurrencyPack {
	return &CurrencyPack{
		identity: identity{id: id, name: name},
		stack:    stack{max: 999},
		currency: currency,
		amount:   amount,
		ledger:   ledger,
	}
}

// ConsumePerUse 一次拆一个。
func (c *CurrencyPack) ConsumePerUse() int64 { return 1 }

// Use 加币。
func (c *CurrencyPack) Use(ctx context.Context, owner string) error {
	return c.ledger.Add(ctx, owner, c.currency, c.amount)
}

// ============================================================
// 随机宝箱
// 能力：可使用 + 可堆叠
//
// 它依赖「发放器」去发出奖品，而发放器就是引擎本身。
// 这个先有鸡还是先有蛋的问题由 engine.LateGranter 解决：
// 先造空壳交给宝箱，引擎构造完成后再 Bind 进去。
// ============================================================

// ChestEntry 宝箱的一档奖品。
type ChestEntry struct {
	// ItemID 奖品道具。
	ItemID string
	// Count 奖品数量。
	Count int64
	// Weight 权重，越大越容易抽中。
	Weight int64
}

// TreasureChest 随机宝箱：按权重抽一档奖品发给玩家。
type TreasureChest struct {
	identity
	display
	stack
	entries []ChestEntry
	total   int64
	granter port.Granter
	rand    port.Rand
}

// NewTreasureChest 构造宝箱；entries 至少一档，权重须为正。
func NewTreasureChest(id, name string, rarity int, entries []ChestEntry,
	granter port.Granter, rand port.Rand) *TreasureChest {
	var total int64
	for _, e := range entries {
		if e.Weight > 0 {
			total += e.Weight
		}
	}
	return &TreasureChest{
		identity: identity{id: id, name: name},
		display:  display{priority: 30, rarity: rarity},
		stack:    stack{max: 99},
		entries:  entries,
		total:    total,
		granter:  granter,
		rand:     rand,
	}
}

// ConsumePerUse 开一次消耗一个。
func (c *TreasureChest) ConsumePerUse() int64 { return 1 }

// Use 抽一档奖品并发放。
// 连开多次由引擎重复调用本方法完成，每次都是独立的一抽。
func (c *TreasureChest) Use(ctx context.Context, owner string) error {
	if c.total <= 0 || len(c.entries) == 0 {
		return fmt.Errorf("%w: 宝箱 %s 未配置奖品", item.ErrInvalidParam, c.id)
	}
	n := c.rand(c.total)
	var acc int64
	for _, e := range c.entries {
		if e.Weight <= 0 {
			continue
		}
		acc += e.Weight
		if n < acc {
			return c.granter.Grant(ctx, owner, e.ItemID, e.Count)
		}
	}
	last := c.entries[len(c.entries)-1]
	return c.granter.Grant(ctx, owner, last.ItemID, last.Count)
}
