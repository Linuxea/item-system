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
	Exec(ctx context.Context, rt Runtime) error
}

type GrantFunc func(ctx context.Context, owner, templateID string, count int64, reason string) error

type Runtime struct {
	Owner  string
	Ledger Ledger
	Banner BannerBroadcaster
	Grant  GrantFunc
	Rand   func(n int64) int64
}

type AddCurrency struct {
	Currency string
	Amount   int64
}

func (c AddCurrency) Kind() string { return KindAddCurrency }

func (c AddCurrency) Exec(ctx context.Context, rt Runtime) error {
	if rt.Ledger == nil {
		return nil
	}
	return rt.Ledger.Add(ctx, rt.Owner, c.Currency, c.Amount)
}

type GrantItem struct {
	TemplateID string
	Count      int64
}

func (c GrantItem) Kind() string { return KindGrantItem }

func (c GrantItem) Exec(ctx context.Context, rt Runtime) error {
	if rt.Grant == nil {
		return nil
	}
	return rt.Grant(ctx, rt.Owner, c.TemplateID, c.Count, KindGrantItem)
}

type RandomEntry struct {
	TemplateID string
	Count      int64
	Weight     int64
}

type RandomGrant struct {
	Entries []RandomEntry
}

func (c RandomGrant) Kind() string { return KindRandomGrant }

func (c RandomGrant) Exec(ctx context.Context, rt Runtime) error {
	if rt.Grant == nil {
		return nil
	}
	entry, ok := c.pick(rt.Rand)
	if !ok {
		return nil
	}
	return rt.Grant(ctx, rt.Owner, entry.TemplateID, entry.Count, KindRandomGrant)
}

func (c RandomGrant) pick(rand func(n int64) int64) (RandomEntry, bool) {
	if len(c.Entries) == 0 {
		return RandomEntry{}, false
	}
	var total int64
	for _, e := range c.Entries {
		total += e.Weight
	}
	if total <= 0 || rand == nil {
		return RandomEntry{}, false
	}
	n := rand(total)
	for _, e := range c.Entries {
		if n < e.Weight {
			return e, true
		}
		n -= e.Weight
	}
	return c.Entries[len(c.Entries)-1], true
}

type BroadcastBanner struct {
	Duration time.Duration
	Text     string
	ParamKey string
}

func (c BroadcastBanner) Kind() string { return KindBroadcastBanner }

func (c BroadcastBanner) Exec(ctx context.Context, rt Runtime) error {
	if rt.Banner == nil {
		return nil
	}
	return rt.Banner.Broadcast(ctx, rt.Owner, c.Text, c.Duration)
}

type Parametrized interface {
	WithParams(params map[string]any) (Command, error)
}

func (c BroadcastBanner) WithParams(params map[string]any) (Command, error) {
	if c.Text != "" {
		return c, nil
	}
	if c.ParamKey == "" {
		return nil, ErrMissingParam
	}
	text, _ := params[c.ParamKey].(string)
	if text == "" {
		return nil, ErrMissingParam
	}
	c.Text = text
	return c, nil
}

var ErrMissingParam = errors.New("required use param missing")

type BannerBroadcaster interface {
	Broadcast(ctx context.Context, owner, text string, duration time.Duration) error
}

type Ledger interface {
	Add(ctx context.Context, owner, currency string, amount int64) error
}

func Execute(ctx context.Context, rt Runtime, cmds []Command) error {
	for _, cmd := range cmds {
		if err := cmd.Exec(ctx, rt); err != nil {
			return err
		}
	}
	return nil
}

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
