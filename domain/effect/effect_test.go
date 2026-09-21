package effect_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Linuxea/item-system/domain/effect"
)

type ledgerFunc func(ctx context.Context, owner, currency string, amount int64) error

func (f ledgerFunc) Add(ctx context.Context, owner, currency string, amount int64) error {
	return f(ctx, owner, currency, amount)
}

type broadcasterFunc func(ctx context.Context, owner, text string, duration time.Duration) error

func (f broadcasterFunc) Broadcast(ctx context.Context, owner, text string, duration time.Duration) error {
	return f(ctx, owner, text, duration)
}

type granterFunc func(ctx context.Context, owner, templateID string, count int64, reason string) error

func (f granterFunc) GrantEffect(ctx context.Context, owner, templateID string, count int64, reason string) error {
	return f(ctx, owner, templateID, count, reason)
}

func TestMaterializeBindsParamsIntoCommand(t *testing.T) {
	cmds := []effect.Command{
		effect.NewAddCurrency(nil, "hp", 50),
		effect.NewBroadcastBanner(nil, 10*time.Second, "text"),
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
	cmds := []effect.Command{effect.NewBroadcastBanner(nil, 10*time.Second, "text")}
	if _, err := effect.Materialize(cmds, nil); !errors.Is(err, effect.ErrMissingParam) {
		t.Fatalf("expected ErrMissingParam, got %v", err)
	}
	if _, err := effect.Materialize(cmds, map[string]any{"text": ""}); !errors.Is(err, effect.ErrMissingParam) {
		t.Fatalf("empty text should fail, got %v", err)
	}
}

func TestMaterializedCommandIsIdempotent(t *testing.T) {
	materialized := effect.BroadcastBanner{Text: "already set"}
	out, err := effect.Materialize([]effect.Command{materialized}, nil)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if out[0].(effect.BroadcastBanner).Text != "already set" {
		t.Fatalf("materialized command must not be rebound: %+v", out[0])
	}
}

func TestEachCommandUsesOnlyItsOwnDeps(t *testing.T) {
	ctx := context.Background()

	var ledgerOwner, ledgerCurrency string
	var ledgerAmount int64
	var bannerCalls []string
	var grantCalls [][2]string

	addCmd := effect.NewAddCurrency(
		ledgerFunc(func(_ context.Context, owner, currency string, amount int64) error {
			ledgerOwner, ledgerCurrency, ledgerAmount = owner, currency, amount
			return nil
		}),
		"hp", 50,
	)
	grantCmd := effect.NewGrantItem(
		granterFunc(func(_ context.Context, owner, templateID string, count int64, reason string) error {
			grantCalls = append(grantCalls, [2]string{owner, templateID})
			return nil
		}),
		"potion_hp", 2,
	)
	randomCmd := effect.NewRandomGrant(
		granterFunc(func(_ context.Context, owner, templateID string, count int64, reason string) error {
			grantCalls = append(grantCalls, [2]string{owner, templateID})
			return nil
		}),
		func(int64) int64 { return 0 },
		[]effect.RandomEntry{
			{TemplateID: "a", Weight: 9},
			{TemplateID: "b", Weight: 1},
		},
	)
	bannerCmd := effect.NewBroadcastBanner(nil, 10*time.Second, "text")
	bannerCmd.Text = "hello"
	broadcastCmd := effect.NewBroadcastBanner(
		broadcasterFunc(func(_ context.Context, owner, text string, d time.Duration) error {
			bannerCalls = append(bannerCalls, owner+"|"+text)
			return nil
		}),
		10*time.Second, "text",
	)
	broadcastCmd.Text = "hello"

	cmds := []effect.Command{addCmd, grantCmd, randomCmd, broadcastCmd}
	if err := effect.Execute(ctx, "p1", cmds); err != nil {
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
