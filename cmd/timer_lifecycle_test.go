package cmd

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTimerTest(t *testing.T) (work string) {
	t.Helper()
	work = t.TempDir()
	t.Chdir(work)
	initTestConfig(t)
	t.Setenv("CF_TIMER_STATE", filepath.Join(work, "timer.json"))
	t.Setenv("CF_TIMER_DB", filepath.Join(work, "timer.db"))
	oldArgs := Args
	Args = &ParsedArgs{}
	t.Cleanup(func() { Args = oldArgs })
	return
}

func timerRecordEvents(t *testing.T, dir string) []timeEvent {
	t.Helper()
	rec, err := loadProblemTimeRecord(filepath.Join(dir, TimeRecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	return rec.Events
}

func TestTimerStartEndLifecycle(t *testing.T) {
	work := setupTimerTest(t)

	if err := timerStart("", "1136a", "read"); err != nil {
		t.Fatal(err)
	}
	st, err := loadTimeState()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || len(st.Open) != 1 {
		t.Fatalf("unexpected state after start: %+v", st)
	}
	seg := st.Open[0]
	if seg.Problem != "1136a" || !strings.HasSuffix(seg.Dir, filepath.Join("contest", "1136", "a")) {
		t.Fatalf("unexpected segment: %+v", seg)
	}
	if seg.Link == "" {
		t.Fatal("start segment should carry the problem link")
	}

	if err = timerStart("", "1136a", "again"); err == nil {
		t.Fatal("duplicate start must be rejected")
	}
	if err = timerStart("", "1136c", "second"); err != nil {
		t.Fatal(err)
	}
	st, _ = loadTimeState()
	if len(st.Open) != 2 {
		t.Fatalf("parallel starts = %v, want 2", len(st.Open))
	}

	// Make the first segment two seconds long, then end it.
	if _, err = mutateTimeState(false, func(st *timeState, now time.Time) error {
		for i := range st.Open {
			if st.Open[i].Problem == "1136a" {
				st.Open[i].StartElapsedMS -= 2000
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = timerEnd("", "1136a", "solved"); err != nil {
		t.Fatal(err)
	}
	st, _ = loadTimeState()
	if len(st.Open) != 1 || st.Open[0].Problem != "1136c" {
		t.Fatalf("after end: %+v", st.Open)
	}
	events := timerRecordEvents(t, filepath.Join(work, "cf", "contest", "1136", "a"))
	if len(events) != 2 || events[0].Type != TimeEventStart || events[1].Type != TimeEventEnd {
		t.Fatalf("unexpected events: %+v", events)
	}
	if events[1].DurationMS < 2000 {
		t.Fatalf("end duration = %v, want >= 2000", events[1].DurationMS)
	}

	if err = timerEnd("", "1136b", ""); err == nil {
		t.Fatal("end without a matching start must be rejected")
	}
	if err = timerEnd("", "", ""); err == nil {
		t.Fatal("end without specifier must be rejected")
	}
	if err = timerEnd("", "1136c", "done"); err != nil {
		t.Fatal(err)
	}
	st, _ = loadTimeState()
	if len(st.Open) != 0 || !st.Running {
		t.Fatalf("unexpected state after closing all problems: %+v", st)
	}
}

func TestCloseTimeSessionAutoEndsOpen(t *testing.T) {
	work := setupTimerTest(t)
	if err := timerStart("", "1136a", "read"); err != nil {
		t.Fatal(err)
	}
	if err := timerStart("", "1136c", "second"); err != nil {
		t.Fatal(err)
	}
	st, err := closeTimeSession("session summary")
	if err != nil {
		t.Fatal(err)
	}
	if st.Running || len(st.Open) != 0 || st.Summary != "session summary" {
		t.Fatalf("unexpected closed state: %+v", st)
	}
	for _, problem := range []string{"a", "c"} {
		events := timerRecordEvents(t, filepath.Join(work, "cf", "contest", "1136", problem))
		last := events[len(events)-1]
		if last.Type != TimeEventEnd || last.Note != "session summary" {
			t.Fatalf("problem %v end event = %+v", problem, last)
		}
	}
	if _, err = closeTimeSession("again"); err == nil {
		t.Fatal("closing a stopped timer must fail")
	}
}

func TestCloseTimeSessionDefaultNote(t *testing.T) {
	work := setupTimerTest(t)
	if err := timerStart("", "1136a", "read"); err != nil {
		t.Fatal(err)
	}
	if _, err := closeTimeSession(""); err != nil {
		t.Fatal(err)
	}
	events := timerRecordEvents(t, filepath.Join(work, "cf", "contest", "1136", "a"))
	last := events[len(events)-1]
	if last.Type != TimeEventEnd || last.Note != "auto end on quit" {
		t.Fatalf("end event = %+v", last)
	}
}

func TestTimerPauseCommand(t *testing.T) {
	setupTimerTest(t)
	if err := timerPause(); err == nil {
		t.Fatal("pause without a running timer must fail")
	}
	if err := timerStart("", "1136a", "read"); err != nil {
		t.Fatal(err)
	}
	if err := timerPause(); err != nil {
		t.Fatal(err)
	}
	st, _ := loadTimeState()
	if !st.Paused || st.RunningSince != "" {
		t.Fatalf("pause state = %+v", st)
	}
	frozen := st.ElapsedMS
	time.Sleep(5 * time.Millisecond)
	st, _ = loadTimeState()
	if got := st.elapsed(time.Now()); got != frozen {
		t.Fatalf("paused elapsed moved: %v != %v", got, frozen)
	}
	if err := timerPause(); err != nil {
		t.Fatal(err)
	}
	st, _ = loadTimeState()
	if st.Paused || st.RunningSince == "" {
		t.Fatalf("resume state = %+v", st)
	}
}

func TestAppendAutoTimeEventMatching(t *testing.T) {
	work := setupTimerTest(t)
	if appendAutoTimeEvent(TimeEventSubmit, "verdict OK") {
		t.Fatal("event without a timer must not be recorded")
	}
	if err := timerStart("", "1136a", "read"); err != nil {
		t.Fatal(err)
	}
	// cwd is work, not the problem folder: no match.
	if appendAutoTimeEvent(TimeEventSubmit, "verdict OK submission 1") {
		t.Fatal("event outside the problem folder must not be recorded")
	}
	t.Chdir(filepath.Join(work, "cf", "contest", "1136", "a"))
	if !appendAutoTimeEvent(TimeEventSubmit, "verdict OK submission 2") {
		t.Fatal("event inside the problem folder was not recorded")
	}
	events := timerRecordEvents(t, filepath.Join(work, "cf", "contest", "1136", "a"))
	last := events[len(events)-1]
	if last.Type != TimeEventSubmit || last.Verdict != "OK" || last.Note != "verdict OK submission 2" {
		t.Fatalf("submit event = %+v", last)
	}
}

func TestNormalizeTimerCommand(t *testing.T) {
	cases := map[string]string{
		"s":      "start",
		"S":      "start",
		"start":  "start",
		"e":      "end",
		"end":    "end",
		"p":      "pause",
		"pause":  "pause",
		"q":      "q",
		"exit":   "q",
		"Q":      "q",
		"status": "status",
		"open":   "open",
		"foo":    "foo",
	}
	for in, want := range cases {
		if got := normalizeTimerCommand(in); got != want {
			t.Errorf("normalizeTimerCommand(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimerStatePathEnv(t *testing.T) {
	t.Setenv("CF_TIMER_STATE", "/tmp/custom-timer.json")
	if got := timerStatePath(); got != "/tmp/custom-timer.json" {
		t.Fatalf("timerStatePath = %q", got)
	}
	t.Setenv("CF_TIMER_STATE", "")
	got := timerStatePath()
	if !strings.HasSuffix(got, filepath.Join(".cf", "timer.json")) {
		t.Fatalf("default timerStatePath = %q", got)
	}
}

func TestAddRecordPath(t *testing.T) {
	records := addRecordPath(nil, "/tmp/a/count_time.json")
	records = addRecordPath(records, "/tmp/a/count_time.json")
	records = addRecordPath(records, "/tmp/b/count_time.json")
	if len(records) != 2 || records[0] != "/tmp/a/count_time.json" || records[1] != "/tmp/b/count_time.json" {
		t.Fatalf("addRecordPath = %v", records)
	}
}

func TestFormatEventTime(t *testing.T) {
	tm := time.Date(2026, 11, 5, 15, 4, 5, 0, time.Local)
	if got := formatEventTime(tm.Format(timeLayout)); got != "15:04:05" {
		t.Fatalf("formatEventTime = %q", got)
	}
	if got := formatEventTime("broken"); got != "broken" {
		t.Fatalf("invalid formatEventTime = %q", got)
	}
}

func TestEventInSessionZeroStart(t *testing.T) {
	if !eventInSession(timeEvent{Time: "broken"}, time.Time{}) {
		t.Fatal("zero session start should keep every event")
	}
}
