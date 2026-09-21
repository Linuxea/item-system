package effect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/linuxea/item-system/domain/effect"
)

func bannerSpec() effect.BroadcastBanner {
	return effect.BroadcastBanner{Duration: 10 * time.Second, ParamKey: "text"}
}

func TestMaterializeBindsParamsIntoCommand(t *testing.T) {
	cmds := []effect.Command{
		effect.AddCurrency{Currency: "hp", Amount: 50},
		bannerSpec(),
	}
	out, err := effect.Materialize(cmds, map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	got := out[1].(effect.BroadcastBanner)
	if got.Text != "hello" || got.Duration != 10*time.Second {
		t.Fatalf("materialized command wrong: %+v", got)
	}
	if out[0].(effect.AddCurrency).Amount != 50 {
		t.Fatalf("static command should pass through: %+v", out[0])
	}
}

func TestMaterializeFailsBeforeAnyExecution(t *testing.T) {
	cmds := []effect.Command{bannerSpec()}
	if _, err := effect.Materialize(cmds, nil); !errors.Is(err, effect.ErrMissingParam) {
		t.Fatalf("expected ErrMissingParam, got %v", err)
	}
	if _, err := effect.Materialize(cmds, map[string]any{"text": ""}); !errors.Is(err, effect.ErrMissingParam) {
		t.Fatalf("empty text should fail, got %v", err)
	}
}

func TestMaterializedCommandIsIdempotent(t *testing.T) {
	materialized := bannerSpec()
	materialized.Text = "already set"
	out, err := effect.Materialize([]effect.Command{materialized}, nil)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if out[0].(effect.BroadcastBanner).Text != "already set" {
		t.Fatalf("materialized command must not be rebound: %+v", out[0])
	}
}

func TestExecWithRuntime(t *testing.T) {
	ctx := context.Background()

	var ledgerOwner, ledgerCurrency string
	var ledgerAmount int64
	var bannerCalls []string
	var grantCalls [][2]string

	rt := effect.Runtime{
		Owner: "p1",
		Ledger: ledgerFunc(func(_ context.Context, owner, currency string, amount int64) error {
			ledgerOwner, ledgerCurrency, ledgerAmount = owner, currency, amount
			return nil
		}),
		Banner: broadcasterFunc(func(_ context.Context, owner, text string, d time.Duration) error {
			bannerCalls = append(bannerCalls, owner+"|"+text)
			return nil
		}),
		Grant: func(_ context.Context, owner, templateID string, count int64, reason string) error {
			grantCalls = append(grantCalls, [2]string{owner, templateID})
			return nil
		},
		Rand: func(n int64) int64 { return 0 },
	}

	banner := bannerSpec()
	banner.Text = "hello"
	cmds := []effect.Command{
		effect.AddCurrency{Currency: "hp", Amount: 50},
		effect.GrantItem{TemplateID: "potion_hp", Count: 2},
		effect.RandomGrant{Entries: []effect.RandomEntry{
			{TemplateID: "a", Weight: 9},
			{TemplateID: "b", Weight: 1},
		}},
		banner,
	}

	if err := effect.Execute(ctx, rt, cmds); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if ledgerOwner != "p1" || ledgerCurrency != "hp" || ledgerAmount != 50 {
		t.Fatalf("ledger call wrong: %s %s %d", ledgerOwner, ledgerCurrency, ledgerAmount)
	}
	if len(grantCalls) != 2 || grantCalls[0] != [2]string{"p1", "potion_hp"} || grantCalls[1] != [2]string{"p1", "a"} {
		t.Fatalf("grant calls wrong: %+v", grantCalls)
	}
	if len(bannerCalls) != 1 || bannerCalls[0] != "p1|hello" {
		t.Fatalf("banner calls wrong: %+v", bannerCalls)
	}
}

type ledgerFunc func(ctx context.Context, owner, currency string, amount int64) error

func (f ledgerFunc) Add(ctx context.Context, owner, currency string, amount int64) error {
	return f(ctx, owner, currency, amount)
}

type broadcasterFunc func(ctx context.Context, owner, text string, duration time.Duration) error

func (f broadcasterFunc) Broadcast(ctx context.Context, owner, text string, duration time.Duration) error {
	return f(ctx, owner, text, duration)
}
