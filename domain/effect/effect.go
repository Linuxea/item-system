package effect

import (
	"context"
	"errors"
	"time"
)

const (
	KindAddCurrency     = "add_currency"
	KindGrantItem       = "grant_item"
	KindRandomGrant     = "random_grant"
	KindBroadcastBanner = "broadcast_banner"
)

type Command interface {
	Kind() string
	Exec(ctx context.Context, owner string) error
}

type Ledger interface {
	Add(ctx context.Context, owner, currency string, amount int64) error
}

type BannerBroadcaster interface {
	Broadcast(ctx context.Context, owner, text string, duration time.Duration) error
}

type Granter interface {
	GrantEffect(ctx context.Context, owner, templateID string, count int64, reason string) error
}

type AddCurrency struct {
	Currency string
	Amount   int64
	ledger   Ledger
}

func NewAddCurrency(ledger Ledger, currency string, amount int64) AddCurrency {
	return AddCurrency{Currency: currency, Amount: amount, ledger: ledger}
}

func (c AddCurrency) Kind() string { return KindAddCurrency }

func (c AddCurrency) Exec(ctx context.Context, owner string) error {
	if c.ledger == nil {
		return nil
	}
	return c.ledger.Add(ctx, owner, c.Currency, c.Amount)
}

type GrantItem struct {
	TemplateID string
	Count      int64
	granter    Granter
}

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

type RandomEntry struct {
	TemplateID string
	Count      int64
	Weight     int64
}

type RandomGrant struct {
	Entries []RandomEntry
	granter Granter
	rand    func(n int64) int64
}

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
	return c.Entries[len(c.Entries)-1], true
}

type BroadcastBanner struct {
	Text     string
	Duration time.Duration
	paramKey string
	banner   BannerBroadcaster
}

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

type Parametrized interface {
	WithParams(params map[string]any) (Command, error)
}

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

var ErrMissingParam = errors.New("required use param missing")

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

func Execute(ctx context.Context, owner string, cmds []Command) error {
	for _, cmd := range cmds {
		if err := cmd.Exec(ctx, owner); err != nil {
			return err
		}
	}
	return nil
}
