package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/fatih/color"
	"github.com/mattn/go-runewidth"
	"github.com/mitchellh/go-homedir"
)

const replHistoryLimit = 1000

var (
	replStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	replSelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	replDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

// replCommandNames is the completion list of the interactive shell. It is the
// top-level command list plus the timer commands that only exist in the shell.
func replCommandNames() []string {
	names := []string{"start", "end", "pause", "q", "exit", "status"}
	names = append(names, completionCommands...)
	seen := map[string]bool{}
	result := names[:0]
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		result = append(result, name)
	}
	return result
}

var (
	replCommands   = replCommandNames()
	replDBCommands = []string{"sql", "time", "stats", "sync", "path"}
)

func replHistoryPath() string {
	if path := strings.TrimSpace(os.Getenv("CF_HISTORY")); path != "" {
		return path
	}
	path, _ := homedir.Expand("~/.cf/history")
	return path
}

func loadReplHistory() []string {
	b, err := os.ReadFile(replHistoryPath())
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	history := lines[:0]
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			history = append(history, line)
		}
	}
	if len(history) > replHistoryLimit {
		history = history[len(history)-replHistoryLimit:]
	}
	return history
}

func appendReplHistory(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	path := replHistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, line)
}

// captureOutput runs fn with stdout and the color writer redirected to a pipe
// and returns everything written. Timer commands print with fmt and color and
// are otherwise unaware of the TUI.
func captureOutput(fn func()) string {
	oldStdout := os.Stdout
	oldColor := color.Output
	r, w, err := os.Pipe()
	if err != nil {
		fn()
		return ""
	}
	os.Stdout = w
	color.Output = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	fn()
	w.Close()
	os.Stdout = oldStdout
	color.Output = oldColor
	return <-done
}

type replTickMsg time.Time

type replExecDoneMsg struct {
	err error
}

type replModel struct {
	input     textinput.Model
	width     int
	history   []string
	histPos   int
	draft     string
	status    *timeState
	lastNote  int64
	nextTick  time.Time
	compOpen  bool
	compItems []string
	compIndex int
	execing   bool
	closed    bool
	quitting  bool
}

func newReplModel() *replModel {
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = "type a cf command; Tab completes, Up recalls history"
	input.CharLimit = 0
	input.Focus()

	history := loadReplHistory()
	m := &replModel{
		input:   input,
		history: history,
		histPos: len(history),
	}
	m.setWidth(80)
	if st, err := loadTimeState(); err == nil {
		m.status = st
		m.lastNote = st.NoticeSeq
	}
	return m
}

func (m *replModel) setWidth(width int) {
	if width < 20 {
		width = 20
	}
	m.width = width
	m.input.Width = width - runewidth.StringWidth(m.input.Prompt) - 2
	if m.input.Width < 10 {
		m.input.Width = 10
	}
}

func replTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return replTickMsg(t)
	})
}

func (m *replModel) Init() tea.Cmd {
	return replTick()
}

func (m *replModel) setInput(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func (m *replModel) historyPrev() {
	if len(m.history) == 0 || m.histPos == 0 {
		return
	}
	if m.histPos == len(m.history) {
		m.draft = m.input.Value()
	}
	m.histPos--
	m.setInput(m.history[m.histPos])
}

func (m *replModel) historyNext() {
	if m.histPos >= len(m.history) {
		return
	}
	m.histPos++
	if m.histPos == len(m.history) {
		m.setInput(m.draft)
		return
	}
	m.setInput(m.history[m.histPos])
}

// completionCandidates lists the values matching the token at the cursor.
func (m *replModel) completionCandidates() []string {
	runes := []rune(m.input.Value())
	pos := m.input.Position()
	if pos > len(runes) {
		pos = len(runes)
	}
	start := pos
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	token := string(runes[start:pos])
	before := strings.TrimSpace(string(runes[:start]))
	fields := strings.Fields(before)

	var pool []string
	switch {
	case len(fields) == 0:
		pool = replCommands
	case fields[0] == "db" && len(fields) == 1:
		pool = replDBCommands
	case fields[0] == "watch" && len(fields) == 1:
		pool = []string{"all"}
	case (fields[0] == "pull" || fields[0] == "clone") && len(fields) == 1:
		pool = []string{"ac"}
	case fields[0] == "completion" && len(fields) == 1:
		pool = []string{"bash", "zsh", "fish"}
	}

	prefix := strings.ToLower(token)
	var matches []string
	for _, item := range pool {
		if strings.HasPrefix(strings.ToLower(item), prefix) {
			matches = append(matches, item)
		}
	}
	if start == 0 {
		sort.Strings(matches)
	}
	return matches
}

func (m *replModel) refreshCompletions() {
	m.compItems = m.completionCandidates()
	if len(m.compItems) == 0 {
		m.compOpen = false
		m.compIndex = 0
		return
	}
	if m.compIndex >= len(m.compItems) {
		m.compIndex = 0
	}
}

func (m *replModel) openCompletions() {
	m.refreshCompletions()
	if len(m.compItems) > 0 {
		m.compOpen = true
		m.compIndex = 0
	}
}

// applyCompletion replaces the token at the cursor with the selected value.
func (m *replModel) applyCompletion() {
	if !m.compOpen || len(m.compItems) == 0 {
		return
	}
	candidate := m.compItems[m.compIndex]
	runes := []rune(m.input.Value())
	pos := m.input.Position()
	if pos > len(runes) {
		pos = len(runes)
	}
	start := pos
	for start > 0 && !unicode.IsSpace(runes[start-1]) {
		start--
	}
	suffix := string(runes[pos:])
	prefix := string(runes[:start])
	insert := candidate
	// A completed command should be followed by a space so the next token can
	// be completed immediately.
	if start == 0 && !strings.HasSuffix(suffix, " ") {
		insert += " "
	}
	value := prefix + insert + suffix
	m.input.SetValue(value)
	m.input.SetCursor(start + len([]rune(insert)))
	m.compOpen = false
	m.compItems = nil
	m.compIndex = 0
}

func (m *replModel) submit() tea.Cmd {
	line := m.input.Value()
	if strings.TrimSpace(line) == "" {
		m.input.SetValue("")
		return nil
	}
	m.input.SetValue("")
	m.input.CursorEnd()
	m.histPos = len(m.history)
	m.draft = ""
	if len(m.history) == 0 || m.history[len(m.history)-1] != line {
		m.history = append(m.history, line)
		appendReplHistory(line)
	}
	m.compOpen = false
	m.compItems = nil

	args := splitCommandLine(line)
	if len(args) == 0 {
		return nil
	}
	echo := tea.Println(replEcho(line))
	if isTimerMonitorCommand(args[0]) {
		return tea.Sequence(echo, m.runTimerLine(args))
	}
	return tea.Sequence(echo, m.runExternal(args))
}

func replEcho(line string) string {
	return color.CyanString(">") + " " + line
}

func (m *replModel) runTimerLine(args []string) tea.Cmd {
	command, practice, specifier, note := parseTimerArgs(args)
	var runErr error
	output := captureOutput(func() {
		runErr = runTimerCommand(command, practice, specifier, note)
	})
	if cur, err := loadTimeState(); err == nil {
		m.status = cur
		m.lastNote = cur.NoticeSeq
	}
	var cmds []tea.Cmd
	if text := strings.TrimRight(output, "\n"); text != "" {
		cmds = append(cmds, tea.Println(text))
	}
	if runErr != nil {
		cmds = append(cmds, tea.Println(color.RedString(runErr.Error())))
	}
	if command == "q" && runErr == nil {
		m.closed = true
		m.quitting = true
		cmds = append(cmds, tea.Quit)
	}
	return tea.Sequence(cmds...)
}

func (m *replModel) runExternal(args []string) tea.Cmd {
	argv, err := monitorCfArgs(args)
	if err != nil {
		return tea.Println(color.RedString(err.Error()))
	}
	exe, err := os.Executable()
	if err != nil {
		return tea.Println(color.RedString(err.Error()))
	}
	cmd := exec.Command(exe, argv...)
	m.execing = true
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return replExecDoneMsg{err: err}
	})
}

func (m *replModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.setWidth(msg.Width)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "ctrl+d":
			m.input.Blur()
			m.quitting = true
			return m, tea.Sequence(
				tea.Println(color.YellowString("Left the timer running. Run `cf` again and type `q` to close it.")),
				tea.Quit,
			)
		case "tab":
			if m.compOpen {
				m.compIndex = (m.compIndex + 1) % len(m.compItems)
			} else {
				m.openCompletions()
			}
			return m, nil
		case "esc":
			m.compOpen = false
			return m, nil
		case "enter":
			if m.compOpen {
				m.applyCompletion()
				return m, nil
			}
			return m, m.submit()
		case "up":
			if m.compOpen {
				m.compIndex--
				if m.compIndex < 0 {
					m.compIndex = len(m.compItems) - 1
				}
				return m, nil
			}
			m.historyPrev()
			return m, nil
		case "down":
			if m.compOpen {
				m.compIndex = (m.compIndex + 1) % len(m.compItems)
				return m, nil
			}
			m.historyNext()
			return m, nil
		}
		var cmd tea.Cmd
		if msg.Type == tea.KeyRunes && len(msg.Runes) > 1 {
			// A terminal can deliver a few typed characters as one key
			// message. textinput compares the whole rune slice against key
			// names, so "end" or "home" would be taken as a cursor command
			// instead of text. Insert the runes one by one.
			for _, r := range msg.Runes {
				if r == '\r' || r == '\n' || r == '\t' {
					r = ' '
				}
				m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			}
		} else {
			m.input, cmd = m.input.Update(msg)
		}
		if m.compOpen {
			m.refreshCompletions()
		}
		return m, cmd

	case replTickMsg:
		if !m.nextTick.IsZero() && time.Now().Before(m.nextTick) {
			return m, nil
		}
		m.nextTick = time.Now().Add(time.Second)
		var cmds []tea.Cmd
		cur, err := loadTimeState()
		if err == nil {
			m.status = cur
			if cur.NoticeSeq != m.lastNote && cur.Notice != "" {
				m.lastNote = cur.NoticeSeq
				cmds = append(cmds, tea.Println(color.CyanString("Timer: %v", cur.Notice)))
			}
			if !cur.Running && !m.execing {
				if !m.closed {
					summary := captureOutput(func() { printTimeSummary(cur) })
					cmds = append(cmds, tea.Println(strings.TrimRight(summary, "\n")))
				}
				m.quitting = true
				cmds = append(cmds, tea.Quit)
			}
		}
		cmds = append(cmds, replTick())
		return m, tea.Sequence(cmds...)

	case replExecDoneMsg:
		m.execing = false
		var cmds []tea.Cmd
		if msg.err != nil && !strings.Contains(msg.err.Error(), "interrupt") {
			cmds = append(cmds, tea.Println(color.RedString(msg.err.Error())))
		}
		if cur, err := loadTimeState(); err == nil {
			m.status = cur
			m.lastNote = cur.NoticeSeq
			if !cur.Running {
				// The child printed the summary when it closed the timer.
				m.closed = true
				m.quitting = true
				cmds = append(cmds, tea.Quit)
			}
		}
		return m, tea.Sequence(cmds...)
	}
	return m, nil
}

func (m *replModel) View() string {
	if m.quitting {
		return ""
	}
	var lines []string
	if m.status != nil {
		status := fmt.Sprintf("[%v] %v", formatElapsedMS(m.status.elapsed(time.Now())), openProblemNames(m.status))
		if m.status.Paused {
			status += " paused"
		}
		status = runewidth.Truncate(status, m.width-1, "…")
		lines = append(lines, replStatusStyle.Render(status))
	} else {
		lines = append(lines, replDimStyle.Render("Timer unavailable"))
	}
	lines = append(lines, m.input.View())
	if m.compOpen && len(m.compItems) > 0 {
		limit := len(m.compItems)
		if limit > 6 {
			limit = 6
		}
		for i := 0; i < limit; i++ {
			label := "  " + m.compItems[i]
			label = runewidth.Truncate(label, m.width-1, "…")
			if i == m.compIndex {
				lines = append(lines, replSelectedStyle.Render(label))
			} else {
				lines = append(lines, replDimStyle.Render(label))
			}
		}
	}
	return strings.Join(lines, "\n")
}

// runReplTUI opens the Bubble Tea shell. SIGINT is handled manually: raw mode
// turns Ctrl-C into a key, and while a child command owns the terminal the
// signal must be drained so the timer survives.
func runReplTUI() error {
	interrupts := make(chan os.Signal, 8)
	done := make(chan struct{})
	signal.Notify(interrupts, os.Interrupt)
	defer func() {
		signal.Stop(interrupts)
		close(done)
	}()
	go func() {
		for {
			select {
			case <-interrupts:
			case <-done:
				return
			}
		}
	}()

	model := newReplModel()
	if width := terminalWidth(int(os.Stdout.Fd())); width > 0 {
		model.setWidth(width)
	}
	program := tea.NewProgram(model, tea.WithoutSignalHandler())
	_, err := program.Run()
	if err == tea.ErrInterrupted {
		return nil
	}
	return err
}
