package effect

import "context"

const (
	KindAddCurrency = "add_currency"
	KindGrantItem   = "grant_item"
	KindRandomGrant = "random_grant"
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

type Ledger interface {
	Add(ctx context.Context, owner, currency string, amount int64) error
}

type Executor interface {
	Execute(ctx context.Context, owner string, cmds []Command) error
}
