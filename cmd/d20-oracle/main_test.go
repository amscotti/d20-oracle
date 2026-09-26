package main

import (
	"os"
	"path/filepath"
	"testing"
)

// unsetenv clears key for the test and restores it afterward.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	old, ok := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if ok {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

func writeEnvFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	path := writeEnvFile(t, dir, ".env",
		"# comment line\n\nD20_PLAIN=hello\nD20_QUOTED='world'\nD20_EXISTING=fromfile\nBAD LINE\nD20_EMPTYKEY=\n")

	unsetenv(t, "D20_PLAIN")
	unsetenv(t, "D20_QUOTED")
	unsetenv(t, "D20_EMPTYKEY")
	t.Setenv("D20_EXISTING", "keepme")

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("D20_PLAIN"); got != "hello" {
		t.Errorf("D20_PLAIN = %q, want hello", got)
	}
	if got := os.Getenv("D20_QUOTED"); got != "world" {
		t.Errorf("D20_QUOTED = %q, want world (quotes stripped)", got)
	}
	if got := os.Getenv("D20_EXISTING"); got != "keepme" {
		t.Errorf("D20_EXISTING = %q, want keepme (no override)", got)
	}
	if got, ok := os.LookupEnv("D20_EMPTYKEY"); !ok || got != "" {
		t.Errorf("D20_EMPTYKEY = %q, %v; want an empty value that is set", got, ok)
	}
}

func TestLoadEnvFileMissing(t *testing.T) {
	if err := loadEnvFile(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Error("missing file must return an error")
	}
}

func TestLoadEnvFileExportAndQuotes(t *testing.T) {
	dir := t.TempDir()
	path := writeEnvFile(t, dir, ".env",
		"export D20_EXPORTED=hi\nD20_MISMATCHED=\"oops\nD20_INNER=it's fine\nD20_HASHED=value # kept verbatim\n")

	for _, key := range []string{"D20_EXPORTED", "D20_MISMATCHED", "D20_INNER", "D20_HASHED"} {
		unsetenv(t, key)
	}

	if err := loadEnvFile(path); err != nil {
		t.Fatalf("loadEnvFile: %v", err)
	}
	if got := os.Getenv("D20_EXPORTED"); got != "hi" {
		t.Errorf("D20_EXPORTED = %q, want %q", got, "hi")
	}
	if got := os.Getenv("D20_MISMATCHED"); got != `"oops` {
		t.Errorf("D20_MISMATCHED = %q, want it untouched", got)
	}
	if got := os.Getenv("D20_INNER"); got != "it's fine" {
		t.Errorf("D20_INNER = %q", got)
	}
	if got := os.Getenv("D20_HASHED"); got != "value # kept verbatim" {
		t.Errorf("D20_HASHED = %q (no inline-comment stripping by design)", got)
	}
}

func TestEnvFilePaths(t *testing.T) {
	paths := envFilePaths()
	if len(paths) == 0 || paths[0] != ".env" {
		t.Fatalf("envFilePaths = %v, want .env first", paths)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		if filepath.Base(path) != ".env" {
			t.Errorf("path %q does not point at a .env file", path)
		}
		if seen[path] {
			t.Errorf("duplicate path %q", path)
		}
		seen[path] = true
	}
}

func TestResolveAPIKey(t *testing.T) {
	t.Run("environment wins", func(t *testing.T) {
		t.Setenv("TYPESAFE_API_KEY", "from-env")
		dir := t.TempDir()
		path := writeEnvFile(t, dir, ".env", "TYPESAFE_API_KEY=from-file\n")

		got, err := resolveAPIKey([]string{path})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "from-env" {
			t.Errorf("key = %q, want from-env", got)
		}
	})

	t.Run("blank environment falls through", func(t *testing.T) {
		t.Setenv("TYPESAFE_API_KEY", "   ")
		dir := t.TempDir()
		path := writeEnvFile(t, dir, ".env", "TYPESAFE_API_KEY=from-file\n")

		got, err := resolveAPIKey([]string{path})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "from-file" {
			t.Errorf("key = %q, want from-file", got)
		}
	})

	t.Run("first non-blank file wins", func(t *testing.T) {
		unsetenv(t, "TYPESAFE_API_KEY")
		dir := t.TempDir()
		first := writeEnvFile(t, dir, "first.env", "TYPESAFE_API_KEY=first\n")
		second := writeEnvFile(t, dir, "second.env", "TYPESAFE_API_KEY=second\n")

		got, err := resolveAPIKey([]string{first, second})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "first" {
			t.Errorf("key = %q, want first", got)
		}
	})

	t.Run("blank file does not shadow", func(t *testing.T) {
		unsetenv(t, "TYPESAFE_API_KEY")
		dir := t.TempDir()
		blank := writeEnvFile(t, dir, "blank.env", "TYPESAFE_API_KEY=\n")
		second := writeEnvFile(t, dir, "second.env", "TYPESAFE_API_KEY=second\n")

		got, err := resolveAPIKey([]string{blank, second})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "second" {
			t.Errorf("key = %q, want second", got)
		}
	})

	t.Run("whitespace file does not shadow", func(t *testing.T) {
		unsetenv(t, "TYPESAFE_API_KEY")
		dir := t.TempDir()
		blank := writeEnvFile(t, dir, "blank.env", "TYPESAFE_API_KEY=   \n")
		second := writeEnvFile(t, dir, "second.env", "TYPESAFE_API_KEY=second\n")

		got, err := resolveAPIKey([]string{blank, second})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "second" {
			t.Errorf("key = %q, want second", got)
		}
	})

	t.Run("quoted value", func(t *testing.T) {
		unsetenv(t, "TYPESAFE_API_KEY")
		dir := t.TempDir()
		path := writeEnvFile(t, dir, ".env", "TYPESAFE_API_KEY='quoted value'\n")

		got, err := resolveAPIKey([]string{path})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "quoted value" {
			t.Errorf("key = %q, want quoted value", got)
		}
	})

	t.Run("nothing set", func(t *testing.T) {
		unsetenv(t, "TYPESAFE_API_KEY")
		got, err := resolveAPIKey([]string{filepath.Join(t.TempDir(), "missing.env")})
		if err != nil {
			t.Fatalf("resolveAPIKey: %v", err)
		}
		if got != "" {
			t.Errorf("key = %q, want empty", got)
		}
	})
}

func TestUnquote(t *testing.T) {
	cases := map[string]string{
		`"yes"`:      "yes",
		`'yes'`:      "yes",
		`"oops`:      `"oops`,
		`oops"`:      `oops"`,
		`""`:         "",
		`"it's"`:     "it's",
		`it's fine`:  "it's fine",
		`plain`:      "plain",
		``:           "",
		`"a'b"`:      "a'b",
		`'a"b'`:      `a"b`,
		`"'nested'"`: `'nested'`,
	}
	for in, want := range cases {
		if got := unquote(in); got != want {
			t.Errorf("unquote(%q) = %q, want %q", in, got, want)
		}
	}
}
