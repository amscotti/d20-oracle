package reading

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amscotti/d20-oracle/internal/classes"
	"github.com/amscotti/d20-oracle/internal/jev"
)

func choiceAnswer(t *testing.T, probs map[string]float64, conf float64) jev.Answer {
	t.Helper()
	winner := ""
	best := -1.0
	for id, p := range probs {
		if p > best {
			best, winner = p, id
		}
	}
	return jev.Answer{
		Type:          jev.TypeChoice,
		Choice:        &winner,
		Probabilities: probs,
		Confidence:    &conf,
	}
}

func TestFromAnswerPicksWinnerAndTwoRunnersUp(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{
		"wizard": 0.31, "sorcerer": 0.22, "bard": 0.14, "rogue": 0.10, "fighter": 0.23,
	}, 0.62)

	r, err := FromAnswer("Sam", "barista", 17, 6, a)
	if err != nil {
		t.Fatalf("FromAnswer: %v", err)
	}
	if r.Class.ID != "wizard" || r.Class.Probability != 0.31 {
		t.Errorf("winner = %+v, want wizard at 0.31", r.Class)
	}
	if r.Class.Name != "Wizard" || r.Class.Symbol == "" || r.Class.Blurb == "" {
		t.Errorf("winner lacks display data: %+v", r.Class)
	}
	if len(r.RunnersUp) != 2 {
		t.Fatalf("runners-up = %d, want exactly 2", len(r.RunnersUp))
	}
	if r.RunnersUp[0].ID != "fighter" || r.RunnersUp[1].ID != "sorcerer" {
		t.Errorf("runners-up = %s, %s; want fighter, sorcerer", r.RunnersUp[0].ID, r.RunnersUp[1].ID)
	}
	if r.Confidence != 0.62 || r.Roll != 17 || r.Name != "Sam" || r.Profession != "barista" {
		t.Errorf("identity fields wrong: %+v", r)
	}
	wizard, _ := classes.ByID("wizard")
	if r.QuirkRoll != 6 || r.Quirk != wizard.Quirks[5] {
		t.Errorf("quirk = %q at %d, want the winning wizard's sixth quirk", r.Quirk, r.QuirkRoll)
	}
}

func TestFromAnswerTiesBreakByCatalogOrder(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{"bard": 0.5, "barbarian": 0.5}, 0.3)
	r, err := FromAnswer("A", "b", 1, 1, a)
	if err != nil {
		t.Fatalf("FromAnswer: %v", err)
	}
	if r.Class.ID != "barbarian" {
		t.Errorf("tie winner = %s, want barbarian (catalog order)", r.Class.ID)
	}
}

func TestFromAnswerFewerThanThree(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{"monk": 0.9, "druid": 0.1}, 0.5)
	r, err := FromAnswer("A", "b", 20, 6, a)
	if err != nil {
		t.Fatalf("FromAnswer: %v", err)
	}
	if len(r.RunnersUp) != 1 || r.RunnersUp[0].ID != "druid" {
		t.Errorf("runners-up = %+v, want just druid", r.RunnersUp)
	}
}

func TestFromAnswerOneClassHasEmptyRunnersUpArray(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{"monk": 1.0}, 0.5)
	r, err := FromAnswer("A", "b", 10, 3, a)
	if err != nil {
		t.Fatalf("FromAnswer: %v", err)
	}
	if r.RunnersUp == nil {
		t.Error("runners-up must be an empty slice, not nil")
	}
	if len(r.RunnersUp) != 0 {
		t.Errorf("runners-up = %+v, want none", r.RunnersUp)
	}
	encoded, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"runners_up":[]`) {
		t.Errorf("JSON = %s, want runners_up as an empty array", encoded)
	}
}

func TestFromAnswerRejectsUnknownClass(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{"artificer": 1.0}, 0.9)
	if _, err := FromAnswer("A", "b", 5, 2, a); err == nil {
		t.Error("unknown class must be rejected")
	}
	if _, err := FromAnswer("A", "b", 5, 2, jev.Answer{Type: jev.TypeChoice}); err == nil {
		t.Error("empty probabilities must be rejected")
	}
}

func TestFromAnswerRejectsInvalidDice(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{"wizard": 1}, 1)
	for _, dice := range []struct{ fate, quirk int }{
		{0, 1}, {21, 1}, {1, 0}, {1, 7},
	} {
		if _, err := FromAnswer("A", "b", dice.fate, dice.quirk, a); err == nil {
			t.Errorf("d20=%d d6=%d: invalid dice must be rejected", dice.fate, dice.quirk)
		}
	}
}

func TestQuirkRollDoesNotChangeTheClassDecision(t *testing.T) {
	a := choiceAnswer(t, map[string]float64{"bard": 0.7, "wizard": 0.3}, 0.6)
	bard, _ := classes.ByID("bard")
	for face := 1; face <= 6; face++ {
		r, err := FromAnswer("Sam", "musician", 12, face, a)
		if err != nil {
			t.Fatalf("d6=%d: %v", face, err)
		}
		if r.Class.ID != "bard" || r.Confidence != 0.6 || r.Quirk != bard.Quirks[face-1] {
			t.Errorf("d6=%d: quirk must belong to the winning class without changing its verdict: %+v", face, r)
		}
	}
}

func TestRollD20Range(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		n, err := RollD20()
		if err != nil {
			t.Fatalf("RollD20: %v", err)
		}
		if n < 1 || n > 20 {
			t.Fatalf("RollD20 = %d, want 1..20", n)
		}
		seen[n] = true
	}
	if len(seen) < 10 {
		t.Errorf("200 rolls only produced %d distinct faces — suspicious", len(seen))
	}
}

func TestRollD6Range(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 200; i++ {
		n, err := RollD6()
		if err != nil {
			t.Fatalf("RollD6: %v", err)
		}
		if n < 1 || n > 6 {
			t.Fatalf("RollD6 = %d, want 1..6", n)
		}
		seen[n] = true
	}
	if len(seen) != 6 {
		t.Errorf("200 rolls only produced %d of the six faces", len(seen))
	}
}
