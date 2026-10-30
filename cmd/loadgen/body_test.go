package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadBodyFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "request.json")
	want := `{"text":"` + strings.Repeat("hello world ", 15000) + `"}`
	if err := os.WriteFile(p, []byte(want), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readBody("", p)
	if err != nil || got != want {
		t.Fatalf("large body not preserved: len=%d err=%v", len(got), err)
	}
	if _, err := readBody("inline", p); err == nil {
		t.Fatal("expected conflicting inputs to fail")
	}
	if _, err := readBody("", p+".missing"); err == nil {
		t.Fatal("expected missing file to fail")
	}
	if got, err := readBody("inline", ""); err != nil || got != "inline" {
		t.Fatalf("inline body changed: %q %v", got, err)
	}
}
