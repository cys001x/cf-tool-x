package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestReplModel(t *testing.T) *replModel {
	t.Helper()
	setupTimerTest(t)
	t.Setenv("CF_HISTORY", filepath.Join(t.TempDir(), "history"))
	return newReplModel()
}

func TestReplTypedRuneBatch(t *testing.T) {
	m := newTestReplModel(t)
	// Terminals may batch several characters into one key message. "end" must
	// be inserted as text, not treated as the End cursor key.
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("end")})
	if got := m.input.Value(); got != "end" {
		t.Fatalf("value after batched runes = %q, want end", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" 1136a done")})
	if got := m.input.Value(); got != "end 1136a done" {
		t.Fatalf("value after batched runes = %q", got)
	}
}

func TestReplCommandCompletion(t *testing.T) {
	m := newTestReplModel(t)
	m.input.SetValue("su")
	m.input.CursorEnd()

	items := m.completionCandidates()
	found := false
	for _, item := range items {
		if item == "submit" {
			found = true
		}
		if item == "config" {
			t.Fatalf("completions for %q must not contain config: %v", "su", items)
		}
	}
	if !found {
		t.Fatalf("completions for %q must contain submit: %v", "su", items)
	}

	m.openCompletions()
	m.applyCompletion()
	if got := m.input.Value(); got != "submit " {
		t.Fatalf("applied completion = %q, want %q", got, "submit ")
	}
}

func TestReplSubcommandCompletion(t *testing.T) {
	m := newTestReplModel(t)
	m.input.SetValue("db ")
	m.input.CursorEnd()

	items := m.completionCandidates()
	if len(items) != len(replDBCommands) {
		t.Fatalf("db completions = %v, want %v", items, replDBCommands)
	}
	for i, want := range replDBCommands {
		if items[i] != want {
			t.Fatalf("db completions = %v, want %v", items, replDBCommands)
		}
	}

	m.input.SetValue("db st")
	m.input.CursorEnd()
	items = m.completionCandidates()
	if len(items) != 1 || items[0] != "stats" {
		t.Fatalf("db st completions = %v, want [stats]", items)
	}
}

func TestReplHistoryNavigation(t *testing.T) {
	m := newTestReplModel(t)
	m.history = []string{"first", "second"}
	m.histPos = len(m.history)

	m.historyPrev()
	if got := m.input.Value(); got != "second" {
		t.Fatalf("first history entry = %q, want second", got)
	}
	m.historyPrev()
	if got := m.input.Value(); got != "first" {
		t.Fatalf("second history entry = %q, want first", got)
	}
	m.historyNext()
	if got := m.input.Value(); got != "second" {
		t.Fatalf("history next = %q, want second", got)
	}
	m.historyNext()
	if got := m.input.Value(); got != "" {
		t.Fatalf("history draft = %q, want empty", got)
	}
}

func TestReplHistoryPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	t.Setenv("CF_HISTORY", path)

	appendReplHistory("status")
	appendReplHistory("db path")
	appendReplHistory("   ")

	history := loadReplHistory()
	if len(history) != 2 || history[0] != "status" || history[1] != "db path" {
		t.Fatalf("history = %#v", history)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("history file = %v, %v", info, err)
	}
}

func TestReplSubmitAppendsHistory(t *testing.T) {
	m := newTestReplModel(t)
	m.input.SetValue("status")
	m.input.CursorEnd()
	m.submit()

	if len(m.history) != 1 || m.history[0] != "status" {
		t.Fatalf("history after submit = %#v", m.history)
	}
	if !strings.Contains(string(mustRead(t, replHistoryPath())), "status") {
		t.Fatal("submitted line must be persisted")
	}
	if m.input.Value() != "" {
		t.Fatalf("input after submit = %q, want empty", m.input.Value())
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
