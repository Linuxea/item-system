package behavior

import (
	"fmt"
	"time"

	"github.com/linuxea/item-system/domain/effect"
	"github.com/linuxea/item-system/domain/model"
)

const (
	KeyStackable  = "stackable"
	KeyExpirable  = "expirable"
	KeyEquippable = "equippable"
	KeyUsable     = "usable"
	KeyBindable   = "bindable"
	KeyPassive    = "passive"
)

const (
	ExpirePolicyRemove    = "remove"
	ExpirePolicyUnequip   = "unequip"
	ExpirePolicyDowngrade = "downgrade"
)

type Behavior interface {
	Key() string
}

type Stackable struct {
	MaxStack int64
}

func (b Stackable) Key() string { return KeyStackable }

type Expirable struct {
	Duration    time.Duration
	OnExpire    string
	DowngradeTo string
}

func (b Expirable) Key() string { return KeyExpirable }

type Equippable struct {
	Slot     model.SlotType
	Capacity int
}

func (b Equippable) Key() string { return KeyEquippable }

type Usable struct {
	Effects []effect.Command
}

func (b Usable) Key() string { return KeyUsable }

type Bindable struct {
	BindOnPickup bool
}

func (b Bindable) Key() string { return KeyBindable }

type Passive struct {
	Modifiers []model.Modifier
}

func (b Passive) Key() string { return KeyPassive }

type Factory func(cfg map[string]any) (Behavior, error)

type Registry struct {
	factories map[string]Factory
}

func NewRegistry() *Registry {
	r := &Registry{factories: map[string]Factory{}}
	r.Register(KeyStackable, stackableFactory)
	r.Register(KeyExpirable, expirableFactory)
	r.Register(KeyEquippable, equippableFactory)
	r.Register(KeyUsable, usableFactory)
	r.Register(KeyBindable, bindableFactory)
	r.Register(KeyPassive, passiveFactory)
	return r
}

func (r *Registry) Register(key string, f Factory) {
	r.factories[key] = f
}

type Compiled struct {
	Template  *model.ItemTemplate
	behaviors map[string]Behavior
}

func (c *Compiled) Get(key string) (Behavior, bool) {
	b, ok := c.behaviors[key]
	return b, ok
}

func (c *Compiled) Stackable() (Stackable, bool) {
	b, ok := c.behaviors[KeyStackable].(Stackable)
	return b, ok
}

func (c *Compiled) Expirable() (Expirable, bool) {
	b, ok := c.behaviors[KeyExpirable].(Expirable)
	return b, ok
}

func (c *Compiled) Equippable() (Equippable, bool) {
	b, ok := c.behaviors[KeyEquippable].(Equippable)
	return b, ok
}

func (c *Compiled) Usable() (Usable, bool) {
	b, ok := c.behaviors[KeyUsable].(Usable)
	return b, ok
}

func (c *Compiled) Bindable() (Bindable, bool) {
	b, ok := c.behaviors[KeyBindable].(Bindable)
	return b, ok
}

func (c *Compiled) Passive() (Passive, bool) {
	b, ok := c.behaviors[KeyPassive].(Passive)
	return b, ok
}

func (r *Registry) Compile(tpl *model.ItemTemplate) (*Compiled, error) {
	c := &Compiled{Template: tpl, behaviors: map[string]Behavior{}}
	for key, cfg := range tpl.Behaviors {
		f, ok := r.factories[key]
		if !ok {
			return nil, fmt.Errorf("behavior %q of template %q: %w", key, tpl.ID, ErrUnknownBehavior)
		}
		b, err := f(cfg)
		if err != nil {
			return nil, fmt.Errorf("behavior %q of template %q: %w", key, tpl.ID, err)
		}
		c.behaviors[key] = b
	}
	return c, nil
}

var ErrUnknownBehavior = fmt.Errorf("unknown behavior")

func stackableFactory(cfg map[string]any) (Behavior, error) {
	return Stackable{MaxStack: getInt(cfg, "max_stack", 1)}, nil
}

func expirableFactory(cfg map[string]any) (Behavior, error) {
	d, err := getDuration(cfg, "duration")
	if err != nil {
		return nil, err
	}
	policy := getString(cfg, "on_expire", ExpirePolicyRemove)
	switch policy {
	case ExpirePolicyRemove, ExpirePolicyUnequip, ExpirePolicyDowngrade:
	default:
		return nil, fmt.Errorf("invalid on_expire policy %q", policy)
	}
	return Expirable{
		Duration:    d,
		OnExpire:    policy,
		DowngradeTo: getString(cfg, "downgrade_to", ""),
	}, nil
}

func equippableFactory(cfg map[string]any) (Behavior, error) {
	slot := getString(cfg, "slot", "")
	if slot == "" {
		return nil, fmt.Errorf("equippable requires slot")
	}
	return Equippable{
		Slot:     model.SlotType(slot),
		Capacity: int(getInt(cfg, "capacity", 1)),
	}, nil
}

func usableFactory(cfg map[string]any) (Behavior, error) {
	raw, ok := cfg["effects"].([]any)
	if !ok {
		return nil, fmt.Errorf("usable requires effects list")
	}
	cmds := make([]effect.Command, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("effect entry must be a map")
		}
		cmd, err := decodeEffect(m)
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, cmd)
	}
	return Usable{Effects: cmds}, nil
}

func decodeEffect(m map[string]any) (effect.Command, error) {
	kind := getString(m, "kind", "")
	switch kind {
	case effect.KindAddCurrency:
		return effect.AddCurrency{
			Currency: getString(m, "currency", ""),
			Amount:   getInt(m, "amount", 0),
		}, nil
	case effect.KindGrantItem:
		return effect.GrantItem{
			TemplateID: getString(m, "template_id", ""),
			Count:      getInt(m, "count", 1),
		}, nil
	case effect.KindRandomGrant:
		raw, ok := m["entries"].([]any)
		if !ok {
			return nil, fmt.Errorf("random_grant requires entries")
		}
		entries := make([]effect.RandomEntry, 0, len(raw))
		for _, e := range raw {
			em, ok := e.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("random entry must be a map")
			}
			entries = append(entries, effect.RandomEntry{
				TemplateID: getString(em, "template_id", ""),
				Count:      getInt(em, "count", 1),
				Weight:     getInt(em, "weight", 1),
			})
		}
		return effect.RandomGrant{Entries: entries}, nil
	default:
		return nil, fmt.Errorf("unknown effect kind %q", kind)
	}
}

func bindableFactory(cfg map[string]any) (Behavior, error) {
	return Bindable{BindOnPickup: getBool(cfg, "bind_on_pickup", false)}, nil
}

func passiveFactory(cfg map[string]any) (Behavior, error) {
	raw, ok := cfg["modifiers"].([]any)
	if !ok {
		return nil, fmt.Errorf("passive requires modifiers list")
	}
	mods := make([]model.Modifier, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("modifier entry must be a map")
		}
		mods = append(mods, model.Modifier{
			Key:   getString(m, "key", ""),
			Value: m["value"],
		})
	}
	return Passive{Modifiers: mods}, nil
}

func getInt(cfg map[string]any, key string, def int64) int64 {
	if v, ok := cfg[key].(int64); ok {
		return v
	}
	if v, ok := cfg[key].(int); ok {
		return int64(v)
	}
	if v, ok := cfg[key].(float64); ok {
		return int64(v)
	}
	return def
}

func getString(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key].(string); ok {
		return v
	}
	return def
}

func getBool(cfg map[string]any, key string, def bool) bool {
	if v, ok := cfg[key].(bool); ok {
		return v
	}
	return def
}

func getDuration(cfg map[string]any, key string) (time.Duration, error) {
	s := getString(cfg, key, "")
	if s == "" {
		return 0, fmt.Errorf("missing duration")
	}
	return time.ParseDuration(s)
}
