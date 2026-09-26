package jev

import (
	"encoding/json"
	"fmt"
	"math"
)

// Primitives: every question is one of these three shapes.
const (
	TypeNoul   = "noul"
	TypeChoice = "choice"
	TypeScore  = "score"
)

// NoulCriteria pins down what a yes and a no mean when the boundary is subtle.
type NoulCriteria struct {
	True  string `json:"true"`
	False string `json:"false"`
}

// Question is a typed question evaluated against the state. Criteria carries
// the per-type rubric and matches one of the documented shapes:
//
//	noul   -> nil or NoulCriteria
//	choice -> map[string]*string (option -> description, nil = no detail)
//	score  -> []string (ordered level descriptions, at least two)
type Question struct {
	Type         string `json:"type"`
	Instructions string `json:"instructions"`
	Criteria     any    `json:"criteria,omitempty"`
}

// Noul builds a yes/no question.
func Noul(instructions string) Question {
	return Question{Type: TypeNoul, Instructions: instructions}
}

// NoulWithCriteria builds a yes/no question with explicit yes/no meanings.
func NoulWithCriteria(instructions, yesMeaning, noMeaning string) Question {
	return Question{
		Type:         TypeNoul,
		Instructions: instructions,
		Criteria:     NoulCriteria{True: yesMeaning, False: noMeaning},
	}
}

// Choice builds a question that picks one option from options (option ->
// rubric description; an empty description needs no extra detail).
func Choice(instructions string, options map[string]string) Question {
	criteria := make(map[string]*string, len(options))
	for opt, desc := range options {
		if desc == "" {
			criteria[opt] = nil
			continue
		}
		d := desc
		criteria[opt] = &d
	}
	return Question{Type: TypeChoice, Instructions: instructions, Criteria: criteria}
}

// Score builds a question that rates the state along ordered levels.
func Score(instructions string, levels []string) Question {
	return Question{Type: TypeScore, Instructions: instructions, Criteria: levels}
}

// Validate checks the question against the documented request schema.
func (q Question) Validate() error {
	if q.Instructions == "" {
		return fmt.Errorf("jev: question %q: instructions must not be empty", q.Type)
	}
	switch q.Type {
	case TypeNoul:
		switch q.Criteria.(type) {
		case nil, NoulCriteria:
			return nil
		default:
			return fmt.Errorf("jev: noul criteria must be a {true,false} object")
		}
	case TypeChoice:
		crit, ok := q.Criteria.(map[string]*string)
		if !ok || len(crit) == 0 {
			return fmt.Errorf("jev: choice criteria must be a non-empty option map")
		}
		return nil
	case TypeScore:
		levels, ok := q.Criteria.([]string)
		if !ok || len(levels) < 2 {
			return fmt.Errorf("jev: score criteria must be at least two ordered levels")
		}
		return nil
	default:
		return fmt.Errorf("jev: unknown question type %q", q.Type)
	}
}

// Usage reports the tokens the request billed. Output tokens are free on Jev.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Answer is one typed answer, tagged by Type with the fields belonging to
// that type populated. Noul answers carry only Noul; choice answers carry
// Choice, Probabilities and Confidence; score answers carry Score, Legend,
// Probabilities and Confidence.
type Answer struct {
	Type          string
	Noul          *float64
	Choice        *string
	Score         *float64
	Probabilities map[string]float64
	Legend        map[string]string
	Confidence    *float64
}

type rawAnswer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul"`
	Choice        *string            `json:"choice"`
	Score         *float64           `json:"score"`
	Probabilities map[string]float64 `json:"probabilities"`
	Legend        map[string]string  `json:"legend"`
	Confidence    *float64           `json:"confidence"`
}

// UnmarshalJSON decodes the tagged answer union and validates it.
func (a *Answer) UnmarshalJSON(data []byte) error {
	var raw rawAnswer
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("jev: decoding answer: %w", err)
	}
	*a = Answer(raw)
	return a.Validate()
}

// Validate checks the answer against the documented response schema:
// noul in [0,1]; choice and score answers carry probabilities that sum to
// one and a confidence in [0,1]; score answers carry their legend.
func (a Answer) Validate() error {
	switch a.Type {
	case TypeNoul:
		if a.Noul == nil {
			return fmt.Errorf("jev: noul answer missing \"noul\"")
		}
		if err := unitInterval("noul", *a.Noul); err != nil {
			return err
		}
		return nil
	case TypeChoice:
		if a.Choice == nil {
			return fmt.Errorf("jev: choice answer missing \"choice\"")
		}
		if _, ok := a.Probabilities[*a.Choice]; !ok {
			return fmt.Errorf("jev: choice %q is not among the probabilities", *a.Choice)
		}
		return a.validateDistribution("choice")
	case TypeScore:
		if a.Score == nil {
			return fmt.Errorf("jev: score answer missing \"score\"")
		}
		if len(a.Legend) < 2 {
			return fmt.Errorf("jev: score answer needs an ordered legend of at least two levels")
		}
		for level := range a.Probabilities {
			if _, ok := a.Legend[level]; !ok {
				return fmt.Errorf("jev: score probability for level %q missing from legend", level)
			}
		}
		return a.validateDistribution("score")
	default:
		return fmt.Errorf("jev: unknown answer type %q", a.Type)
	}
}

func (a Answer) validateDistribution(kind string) error {
	if len(a.Probabilities) == 0 {
		return fmt.Errorf("jev: %s answer missing probabilities", kind)
	}
	sum := 0.0
	for key, p := range a.Probabilities {
		if err := unitInterval(kind+" probability "+key, p); err != nil {
			return err
		}
		sum += p
	}
	if math.Abs(sum-1) > 0.02 {
		return fmt.Errorf("jev: %s probabilities sum to %.3f, want 1", kind, sum)
	}
	if a.Confidence == nil {
		return fmt.Errorf("jev: %s answer missing \"confidence\"", kind)
	}
	return unitInterval("confidence", *a.Confidence)
}

func unitInterval(name string, v float64) error {
	if math.IsNaN(v) || v < 0 || v > 1 {
		return fmt.Errorf("jev: %s = %v, want a value in [0,1]", name, v)
	}
	return nil
}

// Response is one System One evaluation: one answer per question, keyed by
// the ids the caller chose.
type Response struct {
	Model   string            `json:"model"`
	Answers map[string]Answer `json:"answers"`
	Usage   Usage             `json:"usage"`
}

// Validate checks the envelope itself (answers are validated as they decode).
func (r Response) Validate() error {
	if r.Model == "" {
		return fmt.Errorf("jev: response missing \"model\"")
	}
	if r.Answers == nil {
		return fmt.Errorf("jev: response missing \"answers\"")
	}
	return nil
}

// APIError is a non-2xx response from the TypeSafe API. The error body shape
// is not fully specified, so Message is extracted best-effort from the common
// JSON envelopes and Raw keeps the body for diagnostics.
type APIError struct {
	StatusCode int
	Message    string
	Raw        string
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("jev: API error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("jev: API error %d", e.StatusCode)
}
