package util

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLineModelEditing(t *testing.T) {
	m := newLineModel("> ")
	type step struct {
		msg  tea.Msg
		want string
	}
	steps := []step{
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}}, "1"},
		{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}, "12"},
		{tea.KeyMsg{Type: tea.KeyLeft}, "12"},
		{tea.KeyMsg{Type: tea.KeyBackspace}, "2"},
		{tea.KeyMsg{Type: tea.KeyEnter}, "2"},
	}
	for i, s := range steps {
		_, _ = m.Update(s.msg)
		if got := m.input.Value(); got != s.want {
			t.Fatalf("step %d: value %q want %q (pos %d)", i, got, s.want, m.input.Position())
		}
	}
	if m.value != "2" || m.canceled {
		t.Fatalf("final value %q canceled %v", m.value, m.canceled)
	}
}
