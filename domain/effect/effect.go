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
}

type AddCurrency struct {
	Currency string
	Amount   int64
}

func (c AddCurrency) Kind() string { return KindAddCurrency }

type GrantItem struct {
	TemplateID string
	Count      int64
}

func (c GrantItem) Kind() string { return KindGrantItem }

type RandomEntry struct {
	TemplateID string
	Count      int64
	Weight     int64
}

type RandomGrant struct {
	Entries []RandomEntry
}

func (c RandomGrant) Kind() string { return KindRandomGrant }

type BroadcastBanner struct {
	Duration time.Duration
	Text     string
	ParamKey string
}

func (c BroadcastBanner) Kind() string { return KindBroadcastBanner }

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

type Executor interface {
	Execute(ctx context.Context, owner string, cmds []Command) error
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
