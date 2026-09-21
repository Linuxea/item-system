package profile_test

import (
	"testing"
	"time"

	"github.com/linuxea/item-system/domain/model"
	"github.com/linuxea/item-system/domain/profile"
)

func item(id string, priority, rarity int, at time.Time) profile.DisplayItem {
	return profile.DisplayItem{TemplateID: id, Priority: priority, Rarity: rarity, EquippedAt: at}
}

func TestDefaultSort(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Hour)
	items := []profile.DisplayItem{
		item("flame", 70, 3, t0),
		item("abyss", 70, 5, t1),
		item("gold", 90, 5, t0),
		item("ember", 70, 3, t1),
	}

	got := profile.DefaultSort(items)
	want := []string{"gold", "abyss", "flame", "ember"}
	for i, w := range want {
		if got[i].TemplateID != w {
			t.Fatalf("pos %d: want %s got %s", i, w, got[i].TemplateID)
		}
	}
}

func TestSortRegistryFallbackAndCustom(t *testing.T) {
	r := profile.NewSortRegistry()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	items := []profile.DisplayItem{
		item("a", 90, 1, t0),
		item("b", 10, 5, t0),
	}

	if got := r.Policy("unknown_scene")(items); got[0].TemplateID != "a" {
		t.Fatalf("expected fallback to default sort")
	}

	r.Register("chat_bubble", func(in []profile.DisplayItem) []profile.DisplayItem {
		out := []profile.DisplayItem{}
		for _, it := range in {
			if it.Priority >= 50 {
				out = append(out, it)
			}
		}
		return out
	})
	got := r.Policy("chat_bubble")(items)
	if len(got) != 1 || got[0].TemplateID != "a" {
		t.Fatalf("custom policy filtered wrong: %+v", got)
	}
	_ = model.CategoryBadge
}
