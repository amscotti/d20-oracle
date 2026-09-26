// Package reading turns a Jev choice answer and two honest dice into the
// oracle's verdict: a winning class, a d6 quirk, and the next two class matches.
package reading

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"sort"

	"github.com/amscotti/d20-oracle/internal/classes"
	"github.com/amscotti/d20-oracle/internal/jev"
)

// Pick is one placed class in the verdict.
type Pick struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Symbol      string  `json:"symbol"`
	Probability float64 `json:"probability"`
	Blurb       string  `json:"blurb"`
}

// Reading is the verdict for one pair of dice: the class, its quirk, the next
// two probability picks, and how sure the oracle is about the whole thing.
type Reading struct {
	Name       string  `json:"name"`
	Profession string  `json:"profession"`
	Roll       int     `json:"roll"`
	QuirkRoll  int     `json:"quirk_roll"`
	Quirk      string  `json:"quirk"`
	Class      Pick    `json:"class"`
	RunnersUp  []Pick  `json:"runners_up"`
	Confidence float64 `json:"confidence"`
}

// RollD20 rolls an honest, uniform 1..20 with crypto/rand.
func RollD20() (int, error) {
	return roll(20)
}

// RollD6 rolls an honest, uniform 1..6 with crypto/rand.
func RollD6() (int, error) {
	return roll(6)
}

func roll(sides int) (int, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(sides)))
	if err != nil {
		return 0, fmt.Errorf("reading: rolling d%d: %w", sides, err)
	}
	return int(n.Int64()) + 1, nil
}

// FromAnswer places the classes and selects the winner's quirk with the d6.
// The highest probability wins, with ties broken by catalog order. Ranking by
// probability is authoritative, so a misbehaving "choice" field never
// overrides the distribution.
func FromAnswer(name, profession string, roll, quirkRoll int, a jev.Answer) (Reading, error) {
	if roll < 1 || roll > 20 {
		return Reading{}, fmt.Errorf("reading: d20 roll %d is outside 1..20", roll)
	}
	if quirkRoll < 1 || quirkRoll > 6 {
		return Reading{}, fmt.Errorf("reading: d6 roll %d is outside 1..6", quirkRoll)
	}
	if len(a.Probabilities) == 0 {
		return Reading{}, fmt.Errorf("reading: no probabilities to rank")
	}
	for id := range a.Probabilities {
		if _, ok := classes.ByID(id); !ok {
			return Reading{}, fmt.Errorf("reading: unknown class %q in probabilities", id)
		}
	}

	// Catalog order first, then a stable descending sort — ties keep catalog order.
	ordered := make([]Pick, 0, len(a.Probabilities))
	for _, c := range classes.All {
		if p, ok := a.Probabilities[c.ID]; ok {
			ordered = append(ordered, Pick{
				ID: c.ID, Name: c.Name, Symbol: c.Symbol, Probability: p, Blurb: c.Blurb,
			})
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Probability > ordered[j].Probability
	})

	winner, _ := classes.ByID(ordered[0].ID)
	r := Reading{
		Name: name, Profession: profession, Roll: roll,
		QuirkRoll: quirkRoll, Quirk: winner.Quirks[quirkRoll-1], Class: ordered[0],
		RunnersUp: []Pick{},
	}
	if len(ordered) > 1 {
		r.RunnersUp = ordered[1:min(3, len(ordered))]
	}
	if a.Confidence != nil {
		r.Confidence = *a.Confidence
	}
	return r, nil
}
