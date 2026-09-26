// Command d20-oracle serves the one-page D&D class oracle.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/amscotti/d20-oracle/internal/jev"
	"github.com/amscotti/d20-oracle/internal/server"
)

const version = "0.1.0"

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("d20-oracle", version)
		return
	}

	// TYPESAFE_API_KEY comes from the environment. As a convenience, a .env
	// file fills in whatever is still unset — first from the working
	// directory, then from the directory holding the binary (so running the
	// binary from elsewhere still finds its config). The environment always
	// wins over both files.
	key, err := resolveAPIKey(envFilePaths())
	if err != nil {
		log.Fatalf("d20-oracle: %v", err)
	}
	client := jev.NewClient(key)
	if !client.Configured() {
		slog.Warn("TYPESAFE_API_KEY is not set; the page still serves, but /api/class is disabled and will not contact TypeSafe",
			"hint", "set it in the environment or a .env file, then restart")
	}

	srv, err := server.New(server.JevJudge{Client: client})
	if err != nil {
		log.Fatalf("d20-oracle: %v", err)
	}

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: srv.Routes(),
		// Class requests have a 15s application deadline. Leaving
		// WriteTimeout unset lets the handler return its error card
		// when that deadline expires.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("d20-oracle: %v", err)
	}

	go func() {
		log.Printf("d20-oracle %s listening on %s", version, ln.Addr())
		if err := httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("d20-oracle: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	// Survive the full class deadline so an in-flight reading can finish.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("d20-oracle: shutdown: %v", err)
	}
}

// resolveAPIKey returns the first non-blank TYPESAFE_API_KEY from the
// environment or the .env files, in order. Blank and whitespace-only values
// count as unset, so an empty entry cannot shadow a usable value later in the
// search.
func resolveAPIKey(paths []string) (string, error) {
	if key := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")); key != "" {
		return key, nil
	}
	_ = os.Unsetenv("TYPESAFE_API_KEY")

	for _, path := range paths {
		if err := loadEnvFile(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("loading %s: %w", path, err)
		}
		if key := strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY")); key != "" {
			return key, nil
		}
		_ = os.Unsetenv("TYPESAFE_API_KEY")
	}
	return "", nil
}

// envFilePaths lists the .env locations tried in order: the working
// directory first (invocation-specific), then the binary's own directory
// (installation config). loadEnvFile never overwrites an existing variable,
// so the first non-blank value in search order wins and the environment beats
// both files.
func envFilePaths() []string {
	paths := []string{".env"}
	if exe, err := os.Executable(); err == nil {
		if dir := filepath.Dir(exe); dir != "" && dir != "." {
			paths = append(paths, filepath.Join(dir, ".env"))
		}
	}
	return paths
}

// loadEnvFile parses a minimal KEY=VALUE .env file into the process
// environment. Variables already set in the environment win. Blank lines,
// '#' comments, and lines without '=' are ignored; a leading `export ` is
// stripped, and values wrapped in one matching pair of single or double
// quotes are unwrapped. There is deliberately no inline-comment stripping,
// escape processing, or multiline support — keep secrets simple.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = unquote(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
	return nil
}

// unquote removes one matching pair of surrounding single or double quotes.
// Anything else — including mismatched or interior quotes — is left alone.
func unquote(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
