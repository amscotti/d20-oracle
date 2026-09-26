// Package server hosts the one-page class oracle: a name and a profession in,
// a D&D class reading out.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/amscotti/d20-oracle/internal/classes"
	"github.com/amscotti/d20-oracle/internal/jev"
	"github.com/amscotti/d20-oracle/internal/reading"
	"github.com/amscotti/d20-oracle/web"
)

// Judge is the oracle's brain: one choice over the class catalog per verdict.
type Judge interface {
	Choose(ctx context.Context, state map[string]any, options map[string]string) (jev.Response, error)
}

// JevJudge asks the TypeSafe System One model.
type JevJudge struct {
	Client *jev.Client
}

const questionKey = "class"

const missingAPIKeyMessage = "the oracle is not configured — set TYPESAFE_API_KEY and restart"

const instructions = "Which Dungeons & Dragons 5e class best fits this person? " +
	"Their name and profession lead. Interpret d20_roll as a narrative nudge: " +
	"20 favors a legendary expression of a fitting class; " +
	"1 favors a chaotic, unexpected but still plausible fit; " +
	"middling rolls let the name and profession speak. Every class can be heroic. " +
	"Answer with the class option key."

// Ready reports whether the oracle can service readings.
func (j JevJudge) Ready() bool {
	return j.Client != nil && j.Client.Configured()
}

// Choose asks the one question and insists the answer came back.
func (j JevJudge) Choose(ctx context.Context, state map[string]any, options map[string]string) (jev.Response, error) {
	if !j.Ready() {
		return jev.Response{}, jev.ErrMissingAPIKey
	}
	q := map[string]jev.Question{questionKey: jev.Choice(instructions, options)}
	resp, err := j.Client.SystemOne(ctx, state, q)
	if err != nil {
		return jev.Response{}, err
	}
	if _, ok := resp.Answers[questionKey]; !ok {
		return jev.Response{}, fmt.Errorf("server: response missing the %q answer", questionKey)
	}
	return *resp, nil
}

// Input bounds: names stay short enough to show and cheap to judge.
const (
	maxInputRunes       = 60
	maxBodyBytes        = 1 << 20
	classRequestTimeout = 15 * time.Second
)

// Server serves the page and the class API.
type Server struct {
	judge  Judge
	tmpl   *template.Template
	static fs.FS
	now    func() time.Time
}

// New builds the server around a Judge.
func New(judge Judge) (*Server, error) {
	tmpl, err := template.New("").
		Funcs(template.FuncMap{"pct": pct, "visibleRunnersUp": visibleRunnersUp}).
		ParseFS(web.FS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("server: parsing templates: %w", err)
	}
	static, err := fs.Sub(web.FS, "static")
	if err != nil {
		return nil, fmt.Errorf("server: embedded static fs: %w", err)
	}
	return &Server{
		judge:  judge,
		tmpl:   tmpl,
		static: static,
		now:    time.Now,
	}, nil
}

// Routes wires the mux: the page, the class API, static assets, health.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	static := http.StripPrefix("/static/", http.FileServerFS(s.static))
	mux.HandleFunc("GET /static/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/static/" {
			http.NotFound(w, r)
			return
		}
		static.ServeHTTP(w, r)
	})
	// Not /healthz: Cloud Run's frontend reserves paths ending in "z" and
	// answers them with its own 404 before the request reaches the container.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})
	mux.HandleFunc("GET /{$}", s.handlePage)
	mux.HandleFunc("POST /api/class", s.handleClass)
	return securityHeaders(mux)
}

// securityHeaders sets the baseline headers for every response. The page
// pulls pinned scripts and styles from CDNs, so there is no CSP allowlist
// here — but nothing on this server should be sniffed, framed, or sent as a
// referrer to another origin.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// envelope is the app JSON shape: the reading plus provenance.
type envelope struct {
	reading.Reading
	Model     string     `json:"model,omitempty"`
	Usage     *jev.Usage `json:"usage,omitempty"`
	LatencyMS int64      `json:"latency_ms,omitempty"`
}

func (s *Server) handlePage(w http.ResponseWriter, _ *http.Request) {
	s.render(w, http.StatusOK, "page", map[string]any{})
}

func (s *Server) handleClass(w http.ResponseWriter, r *http.Request) {
	if ready, ok := s.judge.(interface{ Ready() bool }); ok && !ready.Ready() {
		s.renderErr(w, r, http.StatusServiceUnavailable, missingAPIKeyMessage)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	name, profession, err := parseInput(r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			s.renderErr(w, r, http.StatusRequestEntityTooLarge, "that reading is too large")
			return
		}
		s.renderErr(w, r, http.StatusBadRequest, err.Error())
		return
	}

	roll, err := reading.RollD20()
	if err != nil {
		s.failLocal(w, r, err)
		return
	}
	quirkRoll, err := reading.RollD6()
	if err != nil {
		s.failLocal(w, r, err)
		return
	}
	state := map[string]any{
		"name":       name,
		"profession": profession,
		"d20_roll":   roll,
		"context":    "assigning a Dungeons & Dragons 5e class to a real person, for fun",
	}

	start := s.now()
	ctx, cancel := context.WithTimeout(r.Context(), classRequestTimeout)
	defer cancel()
	resp, err := s.judge.Choose(ctx, state, classes.Criteria())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	verdict, ok := resp.Answers[questionKey]
	if !ok {
		s.fail(w, r, fmt.Errorf("server: response missing the %q answer", questionKey))
		return
	}
	rd, err := reading.FromAnswer(name, profession, roll, quirkRoll, verdict)
	if err != nil {
		s.failLocal(w, r, err)
		return
	}

	env := envelope{
		Reading:   rd,
		Model:     resp.Model,
		Usage:     &resp.Usage,
		LatencyMS: s.now().Sub(start).Milliseconds(),
	}

	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(env); err != nil {
			slog.Error("encode response", "err", err)
		}
		return
	}

	c, _ := classes.ByID(rd.Class.ID)
	data := map[string]any{"R": env, "Class": c}
	if !isHTMX(r) {
		s.render(w, http.StatusOK, "page", data)
		return
	}
	s.render(w, http.StatusOK, "results", data)
}

// fail renders the upstream error card with an honest status code (HTMX 4
// swaps 4xx/5xx by default, so the partial lands either way).
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, jev.ErrMissingAPIKey) {
		s.renderErr(w, r, http.StatusServiceUnavailable, missingAPIKeyMessage)
		return
	}
	slog.Error("oracle", "err", err)
	status := http.StatusBadGateway
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	s.renderErr(w, r, status, friendlyError(err))
}

// failLocal reports a failure in the oracle's own reasoning, after a
// successful upstream response.
func (s *Server) failLocal(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("oracle: reading", "err", err)
	s.renderErr(w, r, http.StatusInternalServerError, "the oracle misread the omens")
}

// renderErr writes the error card as HTML, or the {"error":…} envelope when
// the caller asked for JSON (?format=json or Accept: application/json). A
// native form post gets the full page so it still looks like the app.
func (s *Server) renderErr(w http.ResponseWriter, r *http.Request, status int, msg string) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
		return
	}
	if !isHTMX(r) {
		s.render(w, status, "page", map[string]any{"Message": msg})
		return
	}
	s.render(w, status, "error", map[string]any{"Message": msg})
}

// friendlyError turns transport and API failures into something worth
// reading on the page. Operator hints belong in the logs, not the visitor's
// error card.
func friendlyError(err error) string {
	if errors.Is(err, jev.ErrMissingAPIKey) {
		return missingAPIKeyMessage
	}
	var apiErr *jev.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return "the oracle is temporarily unavailable"
		case http.StatusUnprocessableEntity:
			return "the TypeSafe API called the request malformed (422)"
		case http.StatusTooManyRequests, 529:
			return "the TypeSafe API is rate limiting or overloaded — try again in a moment"
		}
		if apiErr.Message != "" {
			return "the TypeSafe API said: " + apiErr.Message
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "the oracle timed out"
	}
	return "could not reach the TypeSafe API"
}

func (s *Server) render(w http.ResponseWriter, status int, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		slog.Error("render template", "template", name, "err", err)
	}
}

func wantsJSON(r *http.Request) bool {
	if r.URL.Query().Get("format") == "json" {
		return true
	}
	for _, part := range strings.Split(r.Header.Get("Accept"), ",") {
		mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(part))
		if err != nil || mediaType != "application/json" {
			continue
		}
		if q, ok := params["q"]; ok {
			quality, err := strconv.ParseFloat(q, 64)
			if err != nil || quality <= 0 {
				continue
			}
		}
		return true
	}
	return false
}

// isHTMX reports whether the request came from HTMX; native form posts get a
// full page instead of a bare partial.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// parseInput pulls name and profession from a form post or a JSON body and
// normalizes them (trimmed, capped at maxInputRunes runes). The body is
// already capped at maxBodyBytes by the handler.
func parseInput(r *http.Request) (name string, profession string, err error) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType == "application/json" {
		var body struct {
			Name       string `json:"name"`
			Profession string `json:"profession"`
		}
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&body); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return "", "", maxErr
			}
			return "", "", fmt.Errorf("the oracle could not read that. send {\"name\":…,\"profession\":…}")
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return "", "", fmt.Errorf("the oracle could not read that. send one JSON object")
		}
		name, profession = body.Name, body.Profession
	} else {
		if err := r.ParseForm(); err != nil {
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				return "", "", maxErr
			}
			return "", "", fmt.Errorf("the oracle could not read that form")
		}
		name, profession = r.PostFormValue("name"), r.PostFormValue("profession")
	}

	name = normalize(name)
	profession = normalize(profession)
	if name == "" || profession == "" {
		return "", "", fmt.Errorf("the oracle needs both a name and a profession")
	}
	return name, profession, nil
}

func normalize(s string) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > maxInputRunes {
		s = string([]rune(s)[:maxInputRunes])
	}
	return s
}

func pct(p float64) string {
	return fmt.Sprintf("%.0f%%", p*100)
}

// visibleRunnersUp hides picks whose displayed chance rounds to zero.
func visibleRunnersUp(picks []reading.Pick) []reading.Pick {
	var visible []reading.Pick
	for _, pick := range picks {
		if pct(pick.Probability) != "0%" {
			visible = append(visible, pick)
		}
	}
	return visible
}
