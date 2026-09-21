package effect_test

import (
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
