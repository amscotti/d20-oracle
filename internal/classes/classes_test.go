package classes

import (
	"strings"
	"testing"
)

func TestCatalogHasTwelveDistinctClasses(t *testing.T) {
	if len(All) != 12 {
		t.Fatalf("catalog has %d classes, want the 12 canonical 5e classes", len(All))
	}
	want := map[string]bool{
		"barbarian": true, "bard": true, "cleric": true, "druid": true,
		"fighter": true, "monk": true, "paladin": true, "ranger": true,
		"rogue": true, "sorcerer": true, "warlock": true, "wizard": true,
	}
	seen := map[string]bool{}
	for _, c := range All {
		if seen[c.ID] {
			t.Errorf("duplicate class id %q", c.ID)
		}
		seen[c.ID] = true
		if !want[c.ID] {
			t.Errorf("unexpected class id %q", c.ID)
		}
		if c.ID == "" || c.Name == "" || c.Symbol == "" || c.Blurb == "" ||
			c.Description == "" || c.Criteria == "" {
			t.Errorf("class %q has an empty field", c.ID)
		}
		if !strings.EqualFold(c.ID, strings.ToLower(c.ID)) {
			t.Errorf("class id %q must be lowercase", c.ID)
		}
		if len(c.Traits) == 0 {
			t.Errorf("class %q has no traits", c.ID)
		}
		quirks := map[string]bool{}
		for face, quirk := range c.Quirks {
			if strings.TrimSpace(quirk) == "" {
				t.Errorf("class %q has no quirk for d6 face %d", c.ID, face+1)
			}
			if quirks[quirk] {
				t.Errorf("class %q repeats quirk %q", c.ID, quirk)
			}
			quirks[quirk] = true
		}
	}
	for id := range want {
		if !seen[id] {
			t.Errorf("missing canonical class %q", id)
		}
	}
}

func TestByID(t *testing.T) {
	c, ok := ByID("wizard")
	if !ok || c.Name != "Wizard" {
		t.Fatalf("ByID(wizard) = %q, %v; want Wizard, true", c.Name, ok)
	}
	if _, ok := ByID("artificer"); ok {
		t.Error("artificer is not in the canonical 12; ByID must miss")
	}
}

func TestCriteriaMatchesCatalog(t *testing.T) {
	m := Criteria()
	if len(m) != len(All) {
		t.Fatalf("Criteria has %d entries, want %d", len(m), len(All))
	}
	for _, c := range All {
		if m[c.ID] != c.Criteria {
			t.Errorf("criteria for %q diverged from the catalog", c.ID)
		}
	}
}
