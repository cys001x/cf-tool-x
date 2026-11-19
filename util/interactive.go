package util

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"golang.org/x/term"
)

var promptColor = color.New(color.FgCyan)

// lineModel is a one-shot line editor. It reads a single line with the
// terminal in raw mode, so arrow keys move the cursor instead of being
// inserted as escape sequences like "^[[D".
type lineModel struct {
	input    textinput.Model
	initCmd  tea.Cmd
	value    string
	canceled bool
}

func newLineModel(prompt string) *lineModel {
	input := textinput.New()
	input.Prompt = promptColor.Sprint(prompt)
	m := &lineModel{input: input}
	m.initCmd = m.input.Focus()
	m.setWidth(terminalColumns())
	return m
}

// setWidth limits the visible value so a long line scrolls horizontally
// instead of wrapping and breaking cursor positioning.
func (m *lineModel) setWidth(columns int) {
	if columns <= 0 {
		m.input.Width = 0
		return
	}
	width := columns - lipgloss.Width(m.input.Prompt) - 1
	if width > 0 {
		m.input.Width = width
	}
}

func terminalColumns() int {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return width
}

func (m *lineModel) Init() tea.Cmd { return m.initCmd }

func (m *lineModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setWidth(msg.Width)
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyEnter:
			m.value = m.input.Value()
			return m, tea.Quit
		case tea.KeyCtrlC:
			m.canceled = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *lineModel) View() string { return m.input.View() }

// lineEditorAvailable reports whether stdin/stdout can host the line editor.
func lineEditorAvailable() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

// ScanlinePrompt prints prompt and reads one line. On an interactive terminal
// arrow keys, Home/End and Delete edit the line; otherwise it falls back to a
// plain line read.
func ScanlinePrompt(prompt string) string {
	// Bytes already buffered by Scanline (e.g. pasted input) must not be lost
	// by switching to the raw-mode editor.
	if !lineEditorAvailable() || stdinReader.Buffered() > 0 {
		promptColor.Print(prompt)
		return Scanline()
	}

	final, err := tea.NewProgram(newLineModel(prompt)).Run()
	if err != nil {
		promptColor.Print(prompt)
		return Scanline()
	}
	m, ok := final.(*lineModel)
	if !ok {
		promptColor.Print(prompt)
		return Scanline()
	}
	if m.canceled {
		fmt.Println()
		fmt.Println("Interrupted.")
		os.Exit(1)
	}
	// The line editor erases its rendering on exit; echo the result so the
	// transcript keeps the prompt and the answer.
	promptColor.Print(prompt)
	fmt.Println(m.value)
	return m.value
}

// ScanlinePromptTrim is ScanlinePrompt with surrounding spaces removed.
func ScanlinePromptTrim(prompt string) string {
	return strings.TrimSpace(ScanlinePrompt(prompt))
}
