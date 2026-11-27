package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestReplModel(t *testing.T) *replModel {
	t.Helper()
	setupTimerTest(t)
	t.Setenv("CF_HISTORY", filepath.Join(t.TempDir(), "history"))
	return newReplModel()
}

func TestReplHistorySearch(t *testing.T) {
	m := newTestReplModel(t)
	m.history = []string{"status", "db path", "git status"}
	m.histPos = len(m.history)

	m.startSearch()
	if got := m.searchMatch(); got != "git status" {
		t.Fatalf("initial search match = %q, want git status", got)
	}
	m.searchOlder()
	if got := m.searchMatch(); got != "db path" {
		t.Fatalf("older match = %q, want db path", got)
	}
	m.searchOlder()
	if got := m.searchMatch(); got != "status" {
		t.Fatalf("oldest match = %q, want status", got)
	}
	m.searchNewer()
	if got := m.searchMatch(); got != "db path" {
		t.Fatalf("newer match = %q, want db path", got)
	}

	m.searchQuery = "st"
	m.searchChanged()
	if got := m.searchMatch(); got != "git status" {
		t.Fatalf("query match = %q, want git status", got)
	}
	m.searchOlder()
	if got := m.searchMatch(); got != "status" {
		t.Fatalf("older query match = %q, want status", got)
	}
}

func TestReplHistorySearchKeys(t *testing.T) {
	m := newTestReplModel(t)
	m.history = []string{"status", "db path", "git status"}
	m.histPos = len(m.history)
	m.setInput("draft")

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !m.searching {
		t.Fatal("ctrl+r must start the reverse search")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("db")})
	if got := m.searchMatch(); got != "db path" {
		t.Fatalf("search match for db = %q, want db path", got)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.searching || m.input.Value() != "draft" {
		t.Fatalf("escape must restore draft: searching=%v value=%q", m.searching, m.input.Value())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("git")})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.searching {
		t.Fatal("enter must close the search")
	}
	if got := m.input.Value(); got != "" {
		t.Fatalf("enter must submit the match and clear input, got %q", got)
	}
	if len(m.history) != 3 {
		t.Fatalf("duplicate history entry appended: %#v", m.history)
	}
}

func TestReplHistorySearchAcceptEdit(t *testing.T) {
	m := newTestReplModel(t)
	m.history = []string{"submit 1136a"}
	m.histPos = 1
	m.startSearch()
	m.Update(tea.KeyMsg{Type: tea.KeyTab})
	if m.searching || m.input.Value() != "submit 1136a" {
		t.Fatalf("tab accept = searching=%v value=%q", m.searching, m.input.Value())
	}
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

func TestReplSubmitHistoryPositionIncludesNewCommand(t *testing.T) {
	m := newTestReplModel(t)
	m.history = []string{"first", "second"}
	m.histPos = len(m.history)
	m.input.SetValue("third")
	m.submit()

	if got := m.histPos; got != 3 {
		t.Fatalf("history position after submit = %d, want 3", got)
	}
	m.historyPrev()
	if got := m.input.Value(); got != "third" {
		t.Fatalf("previous command = %q, want third", got)
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

func TestReplRetimerAsksBeforeEnding(t *testing.T) {
	m := newTestReplModel(t)
	if err := timerStart("", "1136a", "read"); err != nil {
		t.Fatal(err)
	}

	m.setInput("reTimer")
	m.submit()
	if !m.resetPending || m.resetCount != 1 {
		t.Fatalf("expected reset confirmation, pending=%v count=%d", m.resetPending, m.resetCount)
	}
	if st, _ := loadTimeState(); len(st.Open) != 1 {
		t.Fatalf("problems ended before confirmation: %+v", st.Open)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if m.resetPending {
		t.Fatal("cancelling should clear the confirmation")
	}
	if st, _ := loadTimeState(); len(st.Open) != 1 {
		t.Fatalf("cancel must keep open problems: %+v", st.Open)
	}

	m.setInput("reTimer")
	m.submit()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	st, err := loadTimeState()
	if err != nil {
		t.Fatal(err)
	}
	if m.resetPending || len(st.Open) != 0 || st.ElapsedMS != 0 {
		t.Fatalf("reset not applied: pending=%v state=%+v", m.resetPending, st)
	}
}

func TestJobLineWriter(t *testing.T) {
	var lines []string
	w := &jobLineWriter{send: func(line string) { lines = append(lines, line) }}
	w.Write([]byte("Submitted\nverdict "))
	w.Write([]byte("OK\r\npartial"))
	w.Flush()

	want := []string{"Submitted", "verdict OK", "partial"}
	if len(lines) != len(want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("lines = %#v, want %#v", lines, want)
		}
	}
}

func TestStripBackgroundFlag(t *testing.T) {
	args, forced := stripBackgroundFlag([]string{"submit", "1136a", "&"})
	if !forced || len(args) != 2 || args[1] != "1136a" {
		t.Fatalf("stripBackgroundFlag = %v, %v", args, forced)
	}
	if _, forced := stripBackgroundFlag([]string{"submit", "1136a"}); forced {
		t.Fatal("a command without & must not be forced")
	}
}

func TestReplCommandClassification(t *testing.T) {
	if !replRunsInBackground([]string{"submit", "1136a"}) {
		t.Fatal("submit must run in the background by default")
	}
	if replRunsInBackground([]string{"config"}) {
		t.Fatal("config must keep the terminal")
	}
	if replRunsInBackground([]string{"upgrade"}) {
		t.Fatal("upgrade must keep the terminal")
	}
}

func TestReplJobQueueLimit(t *testing.T) {
	m := newTestReplModel(t)
	// Fill the running slots with jobs that never started a process.
	for i := 0; i < maxReplJobs; i++ {
		m.jobs[i] = &replJob{id: i, label: "watch"}
	}
	m.enqueueJob([]string{"submit", "1136a"})

	if len(m.jobs) != maxReplJobs {
		t.Fatalf("running jobs = %d, want %d", len(m.jobs), maxReplJobs)
	}
	if len(m.jobQueue) != 1 || m.jobQueue[0].label != "submit 1136a" {
		t.Fatalf("queue = %#v", m.jobQueue)
	}
	if !strings.Contains(m.jobsView(), "Jobs 5/5 +1 queued") {
		t.Fatalf("jobs view = %q", m.jobsView())
	}

	m.killJobs()
	if len(m.jobs) != 0 || len(m.jobQueue) != 0 {
		t.Fatalf("killJobs left jobs=%d queue=%d", len(m.jobs), len(m.jobQueue))
	}
}

func TestReplJobTracksSubmitVerdict(t *testing.T) {
	job := &replJob{label: "submit 1136a", argv: []string{"submit", "1136a"}, started: time.Now()}
	job.trackLine("\x1b[32m status: \x1b[32mAccepted\x1b[0m")
	if job.lastStatus != "Accepted" {
		t.Fatalf("lastStatus = %q, want Accepted", job.lastStatus)
	}
	text := jobDoneText(job, nil)
	if !strings.Contains(text, "submit 1136a") || !strings.Contains(text, "Accepted") {
		t.Fatalf("summary = %q", text)
	}

	job = &replJob{label: "submit 1136a", argv: []string{"submit", "1136a"}, started: time.Now()}
	job.trackLine(" status: Wrong answer on test 2")
	if got := jobDoneText(job, nil); !strings.Contains(got, "Wrong answer") {
		t.Fatalf("summary = %q", got)
	}
}

func TestReplJobTracksTestSamples(t *testing.T) {
	job := &replJob{label: "test 1136a", argv: []string{"test", "1136a"}, started: time.Now()}
	job.trackLine("\x1b[32mPassed #1 0.123s 1.2MB\x1b[0m")
	job.trackLine("\x1b[31mFailed #2 0.234s 2.1MB\x1b[0m")
	if job.passed != 1 || job.failed != 1 {
		t.Fatalf("passed=%d failed=%d", job.passed, job.failed)
	}
	text := jobDoneText(job, nil)
	if !strings.Contains(text, "1 passed, 1 failed") {
		t.Fatalf("summary = %q", text)
	}

	all := &replJob{label: "test 1136a", argv: []string{"test", "1136a"}, started: time.Now()}
	all.trackLine("Passed #1 0.1s 1MB")
	all.trackLine("Passed #2 0.1s 1MB")
	if got := jobDoneText(all, nil); !strings.Contains(got, "all 2 samples passed") {
		t.Fatalf("summary = %q", got)
	}
}

func TestReplJobSummaryKeepsCommandName(t *testing.T) {
	text := jobDoneText(&replJob{label: "getTag 1136a", argv: []string{"getTag", "1136a"}, started: time.Now()}, nil)
	if !strings.Contains(text, "getTag 1136a") {
		t.Fatalf("summary = %q", text)
	}
	if strings.Contains(text, "[") || strings.Contains(text, "0 ") {
		t.Fatalf("summary must not carry the job id: %q", text)
	}
}
