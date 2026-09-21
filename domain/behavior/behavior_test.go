package behavior_test

import (
	"context"
	"testing"

	"github.com/linuxea/item-system/domain/behavior"
	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/model"
)

func TestCompileBuiltins(t *testing.T) {
	r := behavior.NewRegistry(behavior.Ports{})
	tpl := &model.ItemTemplate{
		ID: "chest",
		Behaviors: map[string]map[string]any{
			behavior.KeyStackable: {"max_stack": int64(10)},
			behavior.KeyUsable: {"effects": []any{
				map[string]any{"kind": "add_currency", "currency": "coin", "amount": int64(5)},
				map[string]any{"kind": "random_grant", "entries": []any{
					map[string]any{"template_id": "a", "count": int64(1), "weight": int64(9)},
					map[string]any{"template_id": "b", "count": int64(2), "weight": int64(1)},
				}},
			}},
		},
	}

	c, err := r.Compile(tpl)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if st, ok := c.Stackable(); !ok || st.MaxStack != 10 {
		t.Fatalf("stackable wrong: %+v", st)
	}
	u, ok := c.Usable()
	if !ok || len(u.Effects) != 2 {
		t.Fatalf("usable wrong: %+v", u)
	}
	if u.Effects[0].(effect.AddCurrency).Amount != 5 {
		t.Fatalf("add_currency wrong: %+v", u.Effects[0])
	}
	rg := u.Effects[1].(effect.RandomGrant)
	if len(rg.Entries) != 2 || rg.Entries[1].Weight != 1 {
		t.Fatalf("random_grant wrong: %+v", rg)
	}
}

func TestCompileUnknownBehavior(t *testing.T) {
	r := behavior.NewRegistry(behavior.Ports{})
	tpl := &model.ItemTemplate{
		ID: "broken",
		Behaviors: map[string]map[string]any{
			"teleport": {"to": "moon"},
		},
	}
	if _, err := r.Compile(tpl); err == nil {
		t.Fatalf("expected error for unknown behavior")
	}
}

func TestCompileInvalidExpiryPolicy(t *testing.T) {
	r := behavior.NewRegistry(behavior.Ports{})
	tpl := &model.ItemTemplate{
		ID: "bad_expiry",
		Behaviors: map[string]map[string]any{
			behavior.KeyExpirable: {"duration": "1h", "on_expire": "explode"},
		},
	}
	if _, err := r.Compile(tpl); err == nil {
		t.Fatalf("expected error for invalid policy")
	}
}

type fixedChecker struct {
	ownerOK string
}

func (c fixedChecker) Satisfied(_ context.Context, owner string, _ behavior.Condition) (bool, error) {
	return owner == c.ownerOK, nil
}

func TestConditionChecksItselfWithInjectedChecker(t *testing.T) {
	r := behavior.NewRegistry(behavior.Ports{Conditions: fixedChecker{ownerOK: "p1"}})
	tpl := &model.ItemTemplate{
		ID: "gated",
		Behaviors: map[string]map[string]any{
			behavior.KeyCondition: {"min_level": int64(30), "requires_relation": "cp"},
		},
	}
	c, err := r.Compile(tpl)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cond, ok := c.Condition()
	if !ok || cond.MinLevel != 30 || cond.RequiresRelation != "cp" {
		t.Fatalf("condition wrong: %+v", cond)
	}
	if met, _ := cond.Met(context.Background(), "p1"); !met {
		t.Fatalf("p1 should pass")
	}
	if met, _ := cond.Met(context.Background(), "p2"); met {
		t.Fatalf("p2 should fail")
	}
}

func TestConditionWithoutCheckerAlwaysPasses(t *testing.T) {
	r := behavior.NewRegistry(behavior.Ports{})
	tpl := &model.ItemTemplate{
		ID: "free",
		Behaviors: map[string]map[string]any{
			behavior.KeyCondition: {"min_level": int64(99)},
		},
	}
	c, _ := r.Compile(tpl)
	cond, _ := c.Condition()
	if met, _ := cond.Met(context.Background(), "anyone"); !met {
		t.Fatalf("nil checker should allow")
	}
}
