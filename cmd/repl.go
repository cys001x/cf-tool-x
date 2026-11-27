package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
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

// maxReplJobs is the number of cf commands that may run at the same time in
// the interactive shell. Further commands wait in a queue.
const maxReplJobs = 5

// replForegroundCommands need the terminal to prompt (login menu, upgrade
// confirmation), so they run with the TUI suspended instead of as jobs.
var replForegroundCommands = map[string]bool{
	"config":  true,
	"upgrade": true,
}

// replJob is one cf command running, or waiting to run, in the background.
type replJob struct {
	id      int
	argv    []string
	label   string
	started time.Time
	cmd     *exec.Cmd
}

type replJobLineMsg struct {
	id   int
	line string
}

type replJobDoneMsg struct {
	id  int
	err error
}

// newReplJobCommand builds the child process of a background job. It is a
// variable so tests can replace it.
var newReplJobCommand = func(argv []string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return exec.Command(exe, argv...), nil
}

// jobLineWriter turns the child's writes into whole lines for the TUI. Child
// commands write prompts and progress without a trailing newline, so a partial
// line is buffered until it ends or the job finishes. Write is safe for
// concurrent calls because stdout and stderr share one writer.
type jobLineWriter struct {
	mu   sync.Mutex
	buf  []byte
	send func(string)
}

// jobLineWriterMax bounds a line that never ends.
const jobLineWriterMax = 4096

func (w *jobLineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		w.emit(string(w.buf[:i]))
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) >= jobLineWriterMax {
		w.emit(string(w.buf))
		w.buf = w.buf[:0]
	}
	return len(p), nil
}

func (w *jobLineWriter) emit(line string) {
	line = strings.TrimRight(line, "\r")
	if w.send != nil {
		w.send(line)
	}
}

// Flush emits a trailing line that has no newline.
func (w *jobLineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(string(w.buf))
		w.buf = w.buf[:0]
	}
}

var (
	replStatusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	replSelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("62")).Foreground(lipgloss.Color("230"))
	replDimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	replSearchStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
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

	jobs      map[int]*replJob
	jobQueue  []*replJob
	nextJobID int
	program   *tea.Program

	searching   bool
	searchQuery string
	searchIndex int
	searchSaved string

	resetPending bool
	resetCount   int
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
		jobs:    map[int]*replJob{},
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

// findHistoryMatch scans history from start towards step and returns the first
// entry containing the current query.
func (m *replModel) findHistoryMatch(start, step int) (int, bool) {
	query := strings.ToLower(m.searchQuery)
	for i := start; i >= 0 && i < len(m.history); i += step {
		if strings.Contains(strings.ToLower(m.history[i]), query) {
			return i, true
		}
	}
	return -1, false
}

func (m *replModel) searchOlder() {
	start := len(m.history) - 1
	if m.searchIndex >= 0 {
		start = m.searchIndex - 1
	}
	if index, ok := m.findHistoryMatch(start, -1); ok {
		m.searchIndex = index
	}
}

func (m *replModel) searchNewer() {
	if m.searchIndex < 0 {
		return
	}
	if index, ok := m.findHistoryMatch(m.searchIndex+1, 1); ok {
		m.searchIndex = index
	}
}

func (m *replModel) searchChanged() {
	m.searchIndex = -1
	m.searchOlder()
}

func (m *replModel) startSearch() {
	if m.searching {
		m.searchOlder()
		return
	}
	m.compOpen = false
	m.compItems = nil
	m.searching = true
	m.searchQuery = ""
	m.searchSaved = m.input.Value()
	m.searchIndex = -1
	m.searchOlder()
}

func (m *replModel) searchMatch() string {
	if m.searchIndex < 0 || m.searchIndex >= len(m.history) {
		return ""
	}
	return m.history[m.searchIndex]
}

// acceptSearch copies the selected history entry into the input. execute also
// submits it immediately, like Enter in a shell's reverse search.
func (m *replModel) acceptSearch(execute bool) tea.Cmd {
	match := m.searchMatch()
	m.searching = false
	m.searchQuery = ""
	m.searchIndex = -1
	if match == "" {
		m.setInput(m.searchSaved)
		return nil
	}
	m.setInput(match)
	if !execute {
		return nil
	}
	return m.submit()
}

func (m *replModel) cancelSearch() {
	m.searching = false
	m.searchQuery = ""
	m.searchIndex = -1
	m.setInput(m.searchSaved)
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
	// Keep the cursor after the newest entry. submit() may append the current
	// command, so setting histPos before the append makes Up skip that entry.
	m.histPos = len(m.history)
	m.compOpen = false
	m.compItems = nil

	args := splitCommandLine(line)
	if len(args) == 0 {
		return nil
	}
	// A trailing "&" forces a command that normally needs the terminal into
	// the background queue. Other commands run in the background by default.
	args, forceBackground := stripBackgroundFlag(args)
	echo := tea.Println(replEcho(line))
	if isTimerMonitorCommand(args[0]) {
		command, _, _, _ := parseTimerArgs(args)
		if command == "retimer" {
			if count, ok := timerResetPromptCount(); ok && count > 0 {
				m.resetPending = true
				m.resetCount = count
				return echo
			}
		}
		return tea.Sequence(echo, m.runTimerLine(args))
	}
	if forceBackground || replRunsInBackground(args) {
		argv, err := monitorCfArgs(args)
		if err != nil {
			return tea.Sequence(echo, tea.Println(color.RedString(err.Error())))
		}
		return tea.Sequence(echo, m.enqueueJob(argv))
	}
	return tea.Sequence(echo, m.runExternal(args))
}

// stripBackgroundFlag removes a trailing "&" argument, which forces a command
// into the background queue.
func stripBackgroundFlag(args []string) ([]string, bool) {
	if len(args) > 1 && args[len(args)-1] == "&" {
		return args[:len(args)-1], true
	}
	return args, false
}

// replRunsInBackground reports whether the command may run without the
// terminal.
func replRunsInBackground(args []string) bool {
	return !replForegroundCommands[strings.ToLower(args[0])]
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

// enqueueJob adds args to the background queue and starts as many jobs as the
// concurrency limit allows.
func (m *replModel) enqueueJob(args []string) tea.Cmd {
	job := &replJob{id: m.nextJobID, argv: args, label: strings.Join(args, " ")}
	m.nextJobID++
	m.jobQueue = append(m.jobQueue, job)

	cmds := m.startQueuedJobs()
	if _, running := m.jobs[job.id]; !running {
		cmds = append(cmds, tea.Println(replDimStyle.Render(
			fmt.Sprintf("job %d queued: %v", job.id, shortJobLabel(job.label)))))
	}
	return tea.Sequence(cmds...)
}

// startQueuedJobs launches queued jobs while the concurrency limit allows.
// Starting a process is fast; waiting for it happens in a goroutine.
func (m *replModel) startQueuedJobs() []tea.Cmd {
	var cmds []tea.Cmd
	for len(m.jobs) < maxReplJobs && len(m.jobQueue) > 0 {
		job := m.jobQueue[0]
		m.jobQueue = m.jobQueue[1:]
		if err := m.startJob(job); err != nil {
			cmds = append(cmds, tea.Println(color.RedString(
				"Cannot start job %d (%v): %v", job.id, shortJobLabel(job.label), err)))
		}
	}
	return cmds
}

// startJob runs one cf command without taking over the terminal. The child's
// stdout and stderr are streamed line by line into the TUI and the completion
// is reported with replJobDoneMsg.
func (m *replModel) startJob(job *replJob) error {
	cmd, err := newReplJobCommand(job.argv)
	if err != nil {
		return err
	}
	writer := &jobLineWriter{send: func(line string) {
		if m.program != nil {
			m.program.Send(replJobLineMsg{id: job.id, line: line})
		}
	}}
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Stdin = nil
	if err = cmd.Start(); err != nil {
		return err
	}
	job.cmd = cmd
	job.started = time.Now()
	m.jobs[job.id] = job

	go func() {
		err := cmd.Wait()
		writer.Flush()
		if m.program != nil {
			m.program.Send(replJobDoneMsg{id: job.id, err: err})
		}
	}()
	return nil
}

// killJobs stops every running job and drops the queue. It runs when the shell
// exits: background work does not outlive the TUI.
func (m *replModel) killJobs() {
	for _, job := range m.jobs {
		if job.cmd != nil && job.cmd.Process != nil {
			job.cmd.Process.Kill()
		}
	}
	m.jobs = map[int]*replJob{}
	m.jobQueue = nil
}

// shortJobLabel bounds a command line in single-line output.
func shortJobLabel(label string) string {
	return runewidth.Truncate(label, 28, "…")
}

// formatJobDuration renders a job's running time compactly.
func formatJobDuration(d time.Duration) string {
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// jobsView renders the running jobs line shown at the bottom of the shell.
func (m *replModel) jobsView() string {
	if len(m.jobs) == 0 && len(m.jobQueue) == 0 {
		return ""
	}
	ids := make([]int, 0, len(m.jobs))
	for id := range m.jobs {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	head := fmt.Sprintf("Jobs %d/%d", len(m.jobs), maxReplJobs)
	if queued := len(m.jobQueue); queued > 0 {
		head += fmt.Sprintf(" +%d queued", queued)
	}
	parts := []string{head}
	for _, id := range ids {
		job := m.jobs[id]
		parts = append(parts, fmt.Sprintf("%d %v %v", id, shortJobLabel(job.label),
			formatJobDuration(time.Since(job.started))))
	}
	return strings.Join(parts, " · ")
}

// jobDoneText is the line printed when a background job finishes.
func jobDoneText(job *replJob, err error) string {
	took := formatJobDuration(time.Since(job.started))
	if err != nil {
		return color.RedString("✗ [%d %v] failed after %v: %v",
			job.id, shortJobLabel(job.label), took, err)
	}
	return color.GreenString("✓ [%d %v] finished in %v",
		job.id, shortJobLabel(job.label), took)
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
		if m.resetPending {
			switch msg.String() {
			case "y", "Y":
				m.resetPending = false
				return m, m.runTimerLine([]string{"reTimer"})
			case "n", "N", "esc", "ctrl+c":
				m.resetPending = false
				return m, tea.Println(color.YellowString("Reset cancelled."))
			}
			return m, nil
		}
		if m.searching {
			switch msg.String() {
			case "ctrl+r", "up":
				m.searchOlder()
				return m, nil
			case "down":
				m.searchNewer()
				return m, nil
			case "enter":
				return m, m.acceptSearch(true)
			case "esc", "ctrl+g", "ctrl+c":
				m.cancelSearch()
				return m, nil
			case "tab", "left", "right":
				return m, m.acceptSearch(false)
			case "backspace", "ctrl+h":
				runes := []rune(m.searchQuery)
				if len(runes) > 0 {
					m.searchQuery = string(runes[:len(runes)-1])
					m.searchChanged()
				}
				return m, nil
			case "ctrl+u":
				m.searchQuery = ""
				m.searchChanged()
				return m, nil
			}
			if msg.Type == tea.KeyRunes {
				var query []rune
				for _, r := range msg.Runes {
					if r == '\r' || r == '\n' || r == '\t' {
						r = ' '
					}
					query = append(query, r)
				}
				m.searchQuery += string(query)
				m.searchChanged()
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+r":
			m.startSearch()
			return m, nil
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
			if !cur.Running && !m.execing && len(m.jobs) == 0 && len(m.jobQueue) == 0 {
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

	case replJobLineMsg:
		prefix := replDimStyle.Render(fmt.Sprintf("[%d]", msg.id))
		if job := m.jobs[msg.id]; job != nil {
			prefix = replDimStyle.Render(fmt.Sprintf("[%d %v]", msg.id, shortJobLabel(job.label)))
		}
		return m, tea.Println(prefix + " " + msg.line)

	case replJobDoneMsg:
		job := m.jobs[msg.id]
		delete(m.jobs, msg.id)
		var cmds []tea.Cmd
		if job != nil {
			cmds = append(cmds, tea.Println(jobDoneText(job, msg.err)))
		}
		cmds = append(cmds, m.startQueuedJobs()...)
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
	if m.searching {
		line := fmt.Sprintf("(reverse-i-search)`%v': %v", m.searchQuery, m.searchMatch())
		lines = append(lines, replSearchStyle.Render(runewidth.Truncate(line, m.width-1, "…")))
	} else if m.resetPending {
		line := fmt.Sprintf("End %v running problem(s) and reset the timer? (y/n)", m.resetCount)
		lines = append(lines, replSearchStyle.Render(runewidth.Truncate(line, m.width-1, "…")))
	} else {
		lines = append(lines, m.input.View())
	}
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
	if jobs := m.jobsView(); jobs != "" {
		lines = append(lines, replDimStyle.Render(runewidth.Truncate(jobs, m.width-1, "…")))
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
	model.program = program
	_, err := program.Run()
	// Closing the shell stops its background jobs instead of leaving orphan
	// cf processes behind.
	model.killJobs()
	if err == tea.ErrInterrupted {
		return nil
	}
	return err
}
