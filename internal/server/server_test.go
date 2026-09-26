package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amscotti/d20-oracle/internal/classes"
	"github.com/amscotti/d20-oracle/internal/jev"
	"github.com/amscotti/d20-oracle/internal/reading"
)

type fakeJudge struct {
	answer      jev.Answer
	err         error
	lastState   map[string]any
	lastOptions map[string]string
	calls       int
	deadline    time.Time
	hasDeadline bool
}

func (f *fakeJudge) Choose(ctx context.Context, state map[string]any, options map[string]string) (jev.Response, error) {
	f.lastState, f.lastOptions = state, options
	f.calls++
	f.deadline, f.hasDeadline = ctx.Deadline()
	if f.err != nil {
		return jev.Response{}, f.err
	}
	return jev.Response{
		Model:   "jev-test",
		Answers: map[string]jev.Answer{questionKey: f.answer},
		Usage:   jev.Usage{InputTokens: 42},
	}, nil
}

func wizardAnswer(t *testing.T) jev.Answer {
	t.Helper()
	winner, conf := "wizard", 0.62
	return jev.Answer{
		Type:   jev.TypeChoice,
		Choice: &winner,
		Probabilities: map[string]float64{
			"wizard": 0.31, "fighter": 0.23, "sorcerer": 0.22, "bard": 0.14, "rogue": 0.10,
		},
		Confidence: &conf,
	}
}

func newTestServer(judge Judge) *Server {
	s, err := New(judge)
	if err != nil {
		panic(err)
	}
	return s
}

func postClass(t *testing.T, s *Server, endpoint, name, profession string) *http.Response {
	t.Helper()
	form := url.Values{"name": {name}, "profession": {profession}}
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec.Result()
}

func get(t *testing.T, s *Server, target string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec.Result()
}

func TestClassHTMLFlow(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	resp := postClass(t, s, "/api/class", "Sam", "barista")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, body)
	}
	for _, want := range []string{
		"Wizard", "Sorcerer", "Fighter", // winner + both runner-ups
		"Other paths", "oracle confidence", "Your rolled quirk",
		"spellbook is your diary", // static lore
		"📖",                       // class symbol
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}

	roll, ok := judge.lastState["d20_roll"].(int)
	if !ok || roll < 1 || roll > 20 {
		t.Errorf("d20_roll = %v, want an int in 1..20", judge.lastState["d20_roll"])
	}
	if judge.lastState["name"] != "Sam" || judge.lastState["profession"] != "barista" {
		t.Errorf("state identity wrong: %v", judge.lastState)
	}
	if len(judge.lastOptions) != 12 {
		t.Errorf("options sent to Jev = %d, want the 12 classes", len(judge.lastOptions))
	}
	if judge.calls != 1 {
		t.Errorf("judge calls = %d, want one decision for both dice", judge.calls)
	}
	if _, sent := judge.lastState["quirk_roll"]; sent {
		t.Error("the d6 must select local flavor rather than affect Jev's class choice")
	}
	if !judge.hasDeadline || time.Until(judge.deadline) > classRequestTimeout {
		t.Error("the judge must have an overall request deadline")
	}
}

func TestClassJSONEnvelope(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	resp := postClass(t, s, "/api/class?format=json", "Sam", "barista")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var env struct {
		Name       string `json:"name"`
		Profession string `json:"profession"`
		Roll       int    `json:"roll"`
		QuirkRoll  int    `json:"quirk_roll"`
		Quirk      string `json:"quirk"`
		Class      struct {
			ID          string  `json:"id"`
			Probability float64 `json:"probability"`
		} `json:"class"`
		RunnersUp  []map[string]any `json:"runners_up"`
		Confidence float64          `json:"confidence"`
		Model      string           `json:"model"`
		LatencyMS  int64            `json:"latency_ms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if env.Class.ID != "wizard" || env.Class.Probability != 0.31 {
		t.Errorf("class = %+v, want wizard at 0.31", env.Class)
	}
	if len(env.RunnersUp) != 2 {
		t.Errorf("runners_up = %d, want 2", len(env.RunnersUp))
	}
	if env.Name != "Sam" || env.Profession != "barista" || env.Roll < 1 || env.Roll > 20 {
		t.Errorf("identity/roll wrong: %+v", env)
	}
	if env.Confidence != 0.62 || env.Model != "jev-test" || env.LatencyMS < 0 {
		t.Errorf("provenance wrong: %+v", env)
	}
	if env.QuirkRoll < 1 || env.QuirkRoll > 6 {
		t.Fatalf("quirk_roll = %d, want 1..6", env.QuirkRoll)
	}
	wizard, _ := classes.ByID("wizard")
	if env.Quirk != wizard.Quirks[env.QuirkRoll-1] {
		t.Errorf("quirk = %q, want the winning wizard's rolled quirk", env.Quirk)
	}
}

func TestMissingFields(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	for _, tc := range []struct{ name, profession string }{
		{"", "barista"}, {"Sam", ""}, {"", ""},
	} {
		resp := postClass(t, s, "/api/class", tc.name, tc.profession)
		body := readBody(t, resp)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%q/%q: status = %d, want 400", tc.name, tc.profession, resp.StatusCode)
		}
		if !strings.Contains(body, "needs both a name and a profession") {
			t.Errorf("%q/%q: body missing the friendly message", tc.name, tc.profession)
		}
	}
	if judge.lastState != nil {
		t.Error("judge must not be consulted on invalid input")
	}
}

func TestJSONBodyAccepted(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	req := httptest.NewRequest(http.MethodPost, "/api/class?format=json",
		strings.NewReader(`{"name":"Riley","profession":"cat sitter"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if judge.lastState["profession"] != "cat sitter" {
		t.Errorf("state = %v", judge.lastState)
	}
}

func TestJudgeErrorRendersCard(t *testing.T) {
	judge := &fakeJudge{err: fmt.Errorf("spirits unreachable")}
	s := newTestServer(judge)

	resp := postClass(t, s, "/api/class", "Sam", "barista")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	if !strings.Contains(body, "could not reach the TypeSafe API") {
		t.Errorf("error card missing the friendly message: %s", body)
	}
}

func TestFriendlyError(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"401 is generic":         {&jev.APIError{StatusCode: 401}, "temporarily unavailable"},
		"403 is generic":         {&jev.APIError{StatusCode: 403}, "temporarily unavailable"},
		"missing key":            {jev.ErrMissingAPIKey, "TYPESAFE_API_KEY"},
		"wrapped missing key":    {fmt.Errorf("choose: %w", jev.ErrMissingAPIKey), "TYPESAFE_API_KEY"},
		"422 names malformed":    {&jev.APIError{StatusCode: 422}, "malformed"},
		"429 names rate limit":   {&jev.APIError{StatusCode: 429}, "rate limiting"},
		"529 names overloaded":   {&jev.APIError{StatusCode: 529}, "overloaded"},
		"other keeps message":    {&jev.APIError{StatusCode: 500, Message: "boom"}, "boom"},
		"canceled names timeout": {context.Canceled, "timed out"},
		"generic":                {fmt.Errorf("spirits unreachable"), "could not reach"},
	}
	for name, tc := range cases {
		if got := friendlyError(tc.err); !strings.Contains(got, tc.want) {
			t.Errorf("%s: friendlyError = %q, want it to contain %q", name, got, tc.want)
		}
	}
}

func TestPageAndHealth(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("page status = %d", rec.Code)
	}
	for _, want := range []string{
		"Roll for class.", "id=\"name\"", "id=\"profession\"",
		"https://unpkg.com/htmx.org@4.0.0/dist/htmx.min.js",
		"https://unpkg.com/alpinejs@3.17.4/dist/cdn.min.js",
		`integrity="sha384-`,
		`crossorigin="anonymous"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/health", nil)
	rec = httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Errorf("health = %d %q", rec.Code, rec.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	for _, target := range []string{"/", "/health", "/static/app.css"} {
		resp := get(t, s, target)
		_ = readBody(t, resp)
		for header, want := range map[string]string{
			"X-Content-Type-Options": "nosniff",
			"X-Frame-Options":        "DENY",
			"Referrer-Policy":        "strict-origin-when-cross-origin",
		} {
			if got := resp.Header.Get(header); got != want {
				t.Errorf("GET %s: %s = %q, want %q", target, header, got, want)
			}
		}
	}

	resp := postClass(t, s, "/api/class", "Sam", "barista")
	_ = readBody(t, resp)
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("POST /api/class: X-Content-Type-Options = %q", got)
	}
}

func TestStaticAssetsAreServed(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	for _, target := range []string{"/static/app.css", "/static/app.js"} {
		resp := get(t, s, target)
		_ = readBody(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status = %d, want 200", target, resp.StatusCode)
		}
	}
}

func TestUnknownPathsAreNotFound(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	for _, target := range []string{"/nope", "/favicon.ico", "/api/class/extra"} {
		resp := get(t, s, target)
		_ = readBody(t, resp)
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want 404", target, resp.StatusCode)
		}
	}
}

func TestJSONErrorsAreJSON(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	// Invalid input with ?format=json: 400 plus the machine-readable error.
	resp := postClass(t, s, "/api/class?format=json", "", "barista")
	var inputErr struct {
		Error string `json:"error"`
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&inputErr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(inputErr.Error, "needs both a name and a profession") {
		t.Errorf("error = %q", inputErr.Error)
	}

	// Upstream failure with ?format=json: 502 plus the friendly error.
	sFail := newTestServer(&fakeJudge{err: fmt.Errorf("spirits unreachable")})
	resp = postClass(t, sFail, "/api/class?format=json", "Sam", "barista")
	var upstreamErr struct {
		Error string `json:"error"`
	}
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	if err := json.NewDecoder(resp.Body).Decode(&upstreamErr); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(upstreamErr.Error, "could not reach the TypeSafe API") {
		t.Errorf("error = %q", upstreamErr.Error)
	}
}

func TestAcceptHeaderSelectsJSON(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	form := url.Values{"name": {"Sam"}, "profession": {"barista"}}
	req := httptest.NewRequest(http.MethodPost, "/api/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	resp := rec.Result()
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("Content-Type = %q, want JSON", ct)
	}
	var env struct {
		Class struct {
			ID string `json:"id"`
		} `json:"class"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if env.Class.ID != "wizard" {
		t.Errorf("class = %q, want wizard", env.Class.ID)
	}
}

func TestNormalizeTruncatesByRune(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	name := strings.Repeat("é", 70) // 2 bytes each: byte truncation would split one
	resp := postClass(t, s, "/api/class", name, "barista")
	_ = readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	got, ok := judge.lastState["name"].(string)
	if !ok {
		t.Fatalf("state name = %v, want a string", judge.lastState["name"])
	}
	if n := len([]rune(got)); n != maxInputRunes {
		t.Errorf("name is %d runes, want exactly %d", n, maxInputRunes)
	}
	for _, r := range got {
		if r == '�' {
			t.Fatalf("name contains a replacement char — a rune was split: %q", got)
		}
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	big := strings.Repeat("x", maxBodyBytes+100)
	resp := postClass(t, s, "/api/class", big, "barista")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413; body: %s", resp.StatusCode, body)
	}
	if judge.lastState != nil {
		t.Error("judge must not be consulted on an oversized body")
	}
}

func TestNativeFormPostGetsFullPage(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	resp := postClass(t, s, "/api/class", "Sam", "barista")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(body, "<!doctype html>") || !strings.Contains(body, "Wizard") {
		t.Error("a native form post should receive the full page with the reading")
	}
}

func TestHTMXRequestGetsPartial(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	form := url.Values{"name": {"Sam"}, "profession": {"barista"}}
	req := httptest.NewRequest(http.MethodPost, "/api/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if strings.Contains(body, "<!doctype html>") {
		t.Error("an HTMX request should receive a partial, not the full page")
	}
	if !strings.Contains(body, "Wizard") {
		t.Error("partial missing the winning class")
	}
}

func TestMissingAPIKeyReturns503WithoutUpstreamCall(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"m","answers":{},"usage":{}}`)
	}))
	defer upstream.Close()

	judge := JevJudge{Client: jev.NewClient("",
		jev.WithBaseURL(upstream.URL),
		jev.WithHTTPClient(upstream.Client()),
	)}
	s := newTestServer(judge)

	resp := postClass(t, s, "/api/class", "Sam", "barista")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "TYPESAFE_API_KEY") {
		t.Errorf("body missing the configuration message: %s", body)
	}

	resp = postClass(t, s, "/api/class?format=json", "Sam", "barista")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("json status = %d, want 503", resp.StatusCode)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Body.Close()
	if !strings.Contains(payload.Error, "TYPESAFE_API_KEY") {
		t.Errorf("json error = %q", payload.Error)
	}

	if calls.Load() != 0 {
		t.Errorf("upstream was called %d times with no API key, want 0", calls.Load())
	}

	resp = get(t, s, "/health")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("health status = %d, want 200", resp.StatusCode)
	}
	_ = readBody(t, resp)
}

func TestJevJudgeChoose(t *testing.T) {
	var gotBody map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"jev-test","answers":{"class":{"type":"choice","choice":"wizard","probabilities":{"wizard":1.0},"confidence":0.9}},"usage":{"input_tokens":1}}`)
	}))
	defer upstream.Close()

	judge := JevJudge{Client: jev.NewClient("test-key",
		jev.WithBaseURL(upstream.URL),
		jev.WithHTTPClient(upstream.Client()),
	)}
	resp, err := judge.Choose(context.Background(),
		map[string]any{"name": "Sam"},
		map[string]string{"wizard": "studious"},
	)
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if resp.Answers[questionKey].Choice == nil || *resp.Answers[questionKey].Choice != "wizard" {
		t.Errorf("answer = %+v", resp.Answers[questionKey])
	}

	if gotBody["model"] != jev.Model {
		t.Errorf("model = %v, want %q", gotBody["model"], jev.Model)
	}
	questions, ok := gotBody["questions"].(map[string]any)
	if !ok {
		t.Fatalf("questions = %v", gotBody["questions"])
	}
	q, ok := questions[questionKey].(map[string]any)
	if !ok {
		t.Fatalf("question %q = %v", questionKey, questions[questionKey])
	}
	if q["type"] != jev.TypeChoice {
		t.Errorf("type = %v, want choice", q["type"])
	}
	criteria, ok := q["criteria"].(map[string]any)
	if !ok || len(criteria) != 1 || criteria["wizard"] != "studious" {
		t.Errorf("criteria = %v", q["criteria"])
	}

	empty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"model":"m","answers":{},"usage":{}}`)
	}))
	defer empty.Close()
	missing := JevJudge{Client: jev.NewClient("k",
		jev.WithBaseURL(empty.URL),
		jev.WithHTTPClient(empty.Client()),
	)}
	if _, err := missing.Choose(context.Background(), map[string]any{}, map[string]string{"wizard": "x"}); err == nil {
		t.Error("a response without the class answer must be rejected")
	}
}

func TestMalformedJSONBody(t *testing.T) {
	for name, body := range map[string]string{
		"truncated":      `{"name":`,
		"type error":     `{"name":42,"profession":"barista"}`,
		"trailing value": `{"name":"Sam","profession":"barista"} trailing`,
	} {
		t.Run(name, func(t *testing.T) {
			judge := &fakeJudge{answer: wizardAnswer(t)}
			s := newTestServer(judge)

			req := httptest.NewRequest(http.MethodPost, "/api/class?format=json", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			s.Routes().ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
			}
			if judge.calls != 0 {
				t.Error("malformed input must not reach the judge")
			}
		})
	}
}

func TestJSONContentTypeIsCaseInsensitive(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	req := httptest.NewRequest(http.MethodPost, "/api/class?format=json",
		strings.NewReader(`{"name":"Riley","profession":"cat sitter"}`))
	req.Header.Set("Content-Type", "Application/JSON")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if judge.lastState["name"] != "Riley" {
		t.Errorf("state = %v", judge.lastState)
	}
}

func TestAcceptQualityZeroStaysHTML(t *testing.T) {
	judge := &fakeJudge{answer: wizardAnswer(t)}
	s := newTestServer(judge)

	form := url.Values{"name": {"Sam"}, "profession": {"barista"}}
	req := httptest.NewRequest(http.MethodPost, "/api/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json;q=0")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("Content-Type = %q, want HTML when JSON is refused", ct)
	}
}

func TestDeadlineReturns504(t *testing.T) {
	s := newTestServer(&fakeJudge{err: context.DeadlineExceeded})

	resp := postClass(t, s, "/api/class?format=json", "Sam", "barista")
	body := readBody(t, resp)
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("status = %d, want 504; body: %s", resp.StatusCode, body)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/class"},
		{http.MethodPost, "/"},
		{http.MethodPost, "/health"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		s.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s %s = %d, want 405", tc.method, tc.path, rec.Code)
		}
	}
}

func TestErrorNegotiationViaAccept(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	form := url.Values{"name": {""}, "profession": {"barista"}}
	req := httptest.NewRequest(http.MethodPost, "/api/class", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("Content-Type = %q, want JSON", ct)
	}
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(payload.Error, "needs both") {
		t.Errorf("error = %q", payload.Error)
	}
}

func TestVisibleRunnersUpHidesRoundedZero(t *testing.T) {
	got := visibleRunnersUp([]reading.Pick{
		{ID: "visible", Probability: 0.2},
		{ID: "hidden", Probability: 0.001},
	})
	if len(got) != 1 || got[0].ID != "visible" {
		t.Errorf("visibleRunnersUp = %+v, want only the visible pick", got)
	}
}

func TestStaticRootIsNotFound(t *testing.T) {
	s := newTestServer(&fakeJudge{})

	resp := get(t, s, "/static/")
	_ = readBody(t, resp)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(data)
}
