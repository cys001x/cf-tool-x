package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cys001x/cf-tool-x/client"
)

func TestTimeStatePauseResume(t *testing.T) {
	now := time.Now()
	st := newTimeState(now.Add(-10 * time.Second))
	if !st.Running || st.Paused {
		t.Fatalf("unexpected new state: %+v", st)
	}
	if got := st.elapsed(now); got < 9000 || got > 11000 {
		t.Fatalf("elapsed should be about 10s, got %v", got)
	}

	st.ElapsedMS = st.elapsed(now)
	st.Paused = true
	st.RunningSince = ""
	frozen := st.elapsed(now.Add(time.Hour))
	if frozen != st.ElapsedMS {
		t.Fatalf("paused timer moved: want %v, got %v", st.ElapsedMS, frozen)
	}

	st.Paused = false
	st.RunningSince = now.Add(time.Hour).Format(timeLayout)
	if got := st.elapsed(now.Add(time.Hour + 5*time.Second)); got < st.ElapsedMS+4999 {
		t.Fatalf("timer did not resume: %v", got)
	}
}

func TestAppendProblemEvent(t *testing.T) {
	t.Setenv("CF_TIMER_DB", filepath.Join(t.TempDir(), "timer.db"))
	dir := t.TempDir()
	ev := timeEvent{Type: TimeEventStart, Time: time.Now().Format(timeLayout), Elapsed: "00:00:01", Note: "read"}
	if err := appendProblemEvent(dir, "1136a", "https://codeforces.com/contest/1136/problem/a", ev); err != nil {
		t.Fatalf("appendProblemEvent: %v", err)
	}
	ev = timeEvent{Type: TimeEventEnd, Time: time.Now().Format(timeLayout), Elapsed: "00:00:02"}
	if err := appendProblemEvent(dir, "1136a", "", ev); err != nil {
		t.Fatalf("appendProblemEvent: %v", err)
	}

	rec, err := loadProblemTimeRecord(filepath.Join(dir, TimeRecordFileName))
	if err != nil {
		t.Fatalf("loadProblemTimeRecord: %v", err)
	}
	if rec.Problem != "1136a" || rec.Link == "" {
		t.Fatalf("unexpected record: %+v", rec)
	}
	if len(rec.Events) != 2 || rec.Events[0].Type != TimeEventStart || rec.Events[1].Type != TimeEventEnd {
		t.Fatalf("unexpected events: %+v", rec.Events)
	}
	if rec.Events[0].Elapsed != "00:00:01" || rec.Events[1].Elapsed != "00:00:02" {
		t.Fatalf("elapsed should be HH:MM:SS: %+v", rec.Events)
	}

	if err = setProblemSummary(dir, "1136a", "", "solved with a segment tree"); err != nil {
		t.Fatalf("setProblemSummary: %v", err)
	}
	rec, err = loadProblemTimeRecord(filepath.Join(dir, TimeRecordFileName))
	if err != nil {
		t.Fatalf("loadProblemTimeRecord: %v", err)
	}
	if rec.Summary != "solved with a segment tree" {
		t.Fatalf("unexpected summary: %q", rec.Summary)
	}
	if rec.Link == "" {
		t.Fatal("link should be preserved")
	}
}

func TestParseTimerArgs(t *testing.T) {
	cases := []struct {
		args     []string
		command  string
		practice string
		spec     string
		note     string
	}{
		{[]string{"start", "1136a", "read", "the", "problem"}, "start", "", "1136a", "read the problem"},
		{[]string{"start", "p_drill", "1136a", "read"}, "start", "p_drill", "1136a", "read"},
		{[]string{"end", "1136a"}, "end", "", "1136a", ""},
		{[]string{"end", "p_drill", "1136a"}, "end", "p_drill", "1136a", ""},
		{[]string{"pause"}, "pause", "", "", ""},
		{[]string{"q", "contest", "done"}, "q", "", "", "contest done"},
		{[]string{"exit", "done"}, "q", "", "", "done"},
		{[]string{"s", "1136a", "read"}, "start", "", "1136a", "read"},
		{[]string{"s", "p_drill", "1136a", "read"}, "start", "p_drill", "1136a", "read"},
		{[]string{"e", "1136a"}, "end", "", "1136a", ""},
		{[]string{"p"}, "pause", "", "", ""},
	}
	for _, c := range cases {
		command, practice, spec, note := parseTimerArgs(c.args)
		if command != c.command || practice != c.practice || spec != c.spec || note != c.note {
			t.Errorf("parseTimerArgs(%v) = (%q, %q, %q, %q), want (%q, %q, %q, %q)",
				c.args, command, practice, spec, note, c.command, c.practice, c.spec, c.note)
		}
	}
}

func TestAppendAutoTimeEvent(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "timer.json")
	t.Setenv("CF_TIMER_STATE", statePath)
	t.Setenv("CF_TIMER_DB", filepath.Join(t.TempDir(), "timer.db"))
	root := t.TempDir()
	dir := filepath.Join(root, "1136", "a")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	st := newTimeState(now.Add(-time.Minute))
	st.Open = []timeSegment{{Specifier: "1136a", Problem: "1136a", Dir: dir, Start: now.Format(timeLayout), Practice: "drill"}}
	st.Records = []string{filepath.Join(dir, TimeRecordFileName)}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, b, 0644); err != nil {
		t.Fatal(err)
	}

	oldArgs := Args
	defer func() { Args = oldArgs }()
	Args = &ParsedArgs{Info: client.Info{
		ContestID:   "1136",
		ProblemType: "contest",
		ProblemID:   "a",
		RootPath:    root,
	}}

	recordTestPassed()

	rec, err := loadProblemTimeRecord(filepath.Join(dir, TimeRecordFileName))
	if err != nil {
		t.Fatalf("loadProblemTimeRecord: %v", err)
	}
	if len(rec.Events) != 1 || rec.Events[0].Type != TimeEventTest {
		t.Fatalf("unexpected events: %+v", rec.Events)
	}
	if rec.Events[0].Practice != "drill" {
		t.Fatalf("practice set was not recorded: %+v", rec.Events[0])
	}

	saved, err := loadTimeState()
	if err != nil {
		t.Fatalf("loadTimeState: %v", err)
	}
	if saved.NoticeSeq == 0 || saved.Notice == "" {
		t.Fatalf("auto event should queue a notice: %+v", saved)
	}
}

func TestEventInSession(t *testing.T) {
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	old := timeEvent{Type: TimeEventStart, Time: start.Add(-time.Second).Format(timeLayout)}
	inSession := timeEvent{Type: TimeEventStart, Time: start.Add(time.Second).Format(timeLayout)}
	if eventInSession(old, start) {
		t.Fatal("event before the session start should be skipped")
	}
	if !eventInSession(inSession, start) {
		t.Fatal("event after the session start should be kept")
	}
	if !eventInSession(timeEvent{Time: "broken"}, start) {
		t.Fatal("unparseable event should be kept")
	}
}

func TestFormatElapsedMS(t *testing.T) {
	cases := map[int64]string{
		0:       "00:00:00",
		59999:   "00:00:59",
		60000:   "00:01:00",
		3661000: "01:01:01",
		-5:      "00:00:00",
	}
	for in, want := range cases {
		if got := formatElapsedMS(in); got != want {
			t.Errorf("formatElapsedMS(%v): want %v, got %v", in, want, got)
		}
	}
}
