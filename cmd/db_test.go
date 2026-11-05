package cmd

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestIndexProblemRecordDB(t *testing.T) {
	t.Setenv("CF_TIMER_DB", filepath.Join(t.TempDir(), "timer.db"))
	dir := t.TempDir()
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	link := "https://codeforces.com/contest/1136/problem/a"

	if err := appendProblemEvent(dir, "1136a", link, timeEvent{
		Type: TimeEventStart, Time: start.Format(timeLayout), Elapsed: "00:00:00", Note: "read",
	}); err != nil {
		t.Fatal(err)
	}
	if err := appendProblemEvent(dir, "1136a", link, timeEvent{
		Type: TimeEventEnd, Time: start.Add(5 * time.Minute).Format(timeLayout), Elapsed: "00:05:00",
	}); err != nil {
		t.Fatal(err)
	}
	if err := appendProblemEvent(dir, "1136a", link, timeEvent{
		Type: TimeEventSubmit, Time: start.Add(5 * time.Minute).Format(timeLayout), Elapsed: "00:05:00",
		Verdict: "OK", Note: "verdict OK",
	}); err != nil {
		t.Fatal(err)
	}
	if err := appendProblemEvent(dir, "1136a", link, timeEvent{
		Type: TimeEventTest, Time: start.Add(4 * time.Minute).Format(timeLayout), Elapsed: "00:04:00",
		Note: "all samples passed",
	}); err != nil {
		t.Fatal(err)
	}

	db, err := openTimerDB(true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var key, display, verdict string
	var duration int64
	if err = db.QueryRow(`SELECT problem_key, duration_ms, verdict FROM event WHERE type = 'end'`).
		Scan(&key, &duration, &verdict); err != nil {
		t.Fatal(err)
	}
	if key != "contest:1136:a" || duration != 300000 {
		t.Fatalf("unexpected end event: key=%q duration=%v", key, duration)
	}
	if err = db.QueryRow(`SELECT display FROM problem WHERE key = ?`, key).Scan(&display); err != nil {
		t.Fatal(err)
	}
	if display != "1136a" {
		t.Fatalf("unexpected display: %q", display)
	}
	var status string
	var practice int
	if err = db.QueryRow(`SELECT status, practice FROM problem WHERE key = ?`, key).Scan(&status, &practice); err != nil {
		t.Fatal(err)
	}
	if status != problemStatusAccepted || practice != 0 {
		t.Fatalf("unexpected problem state: status=%q practice=%v", status, practice)
	}
	if err = db.QueryRow(`SELECT verdict FROM event WHERE type = 'submit'`).Scan(&verdict); err != nil {
		t.Fatal(err)
	}
	if verdict != "OK" {
		t.Fatalf("unexpected verdict: %q", verdict)
	}
	var total int64
	if err = db.QueryRow(`SELECT total_ms FROM problem_time WHERE problem_key = ?`, key).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 300000 {
		t.Fatalf("view total = %v, want 300000", total)
	}
}

func TestLegacyEndEventDuration(t *testing.T) {
	t.Setenv("CF_TIMER_DB", filepath.Join(t.TempDir(), "timer.db"))
	dir := t.TempDir()
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.Local)
	rec := &problemTimeRecord{
		Problem: "1136a",
		Link:    "https://codeforces.com/contest/1136/problem/a",
		Events: []timeEvent{
			{Type: TimeEventStart, Time: start.Format(timeLayout), Elapsed: "00:01:00"},
			{Type: TimeEventEnd, Time: start.Add(time.Minute).Format(timeLayout), Elapsed: "00:04:30"},
		},
	}
	b := []byte(`{"problem":"1136a","link":"https://codeforces.com/contest/1136/problem/a","events":[` +
		`{"type":"start","time":"` + rec.Events[0].Time + `","elapsed":"00:01:00"},` +
		`{"type":"end","time":"` + rec.Events[1].Time + `","elapsed":"00:04:30"}]}`)
	if err := writeFileAtomic(filepath.Join(dir, TimeRecordFileName), b); err != nil {
		t.Fatal(err)
	}
	if err := indexProblemRecord(dir); err != nil {
		t.Fatal(err)
	}
	db, err := openTimerDB(true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var duration int64
	if err = db.QueryRow(`SELECT duration_ms FROM event WHERE type = 'end'`).Scan(&duration); err != nil {
		t.Fatal(err)
	}
	if duration != 210000 {
		t.Fatalf("legacy duration = %v, want 210000", duration)
	}
	var status string
	if err = db.QueryRow(`SELECT status FROM problem WHERE key = 'contest:1136:a'`).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != problemStatusDone {
		t.Fatalf("unexpected legacy status: %q", status)
	}
}

func TestProblemColumnsMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "timer.db")
	t.Setenv("CF_TIMER_DB", path)
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(`CREATE TABLE problem (
		key TEXT PRIMARY KEY,
		contest_key TEXT NOT NULL DEFAULT '',
		problem_id TEXT NOT NULL DEFAULT '',
		display TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		link TEXT NOT NULL DEFAULT '',
		dir TEXT NOT NULL DEFAULT '',
		summary TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	db, err := openTimerDB(false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`PRAGMA table_info(problem)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue interface{}
		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if !columns["status"] || !columns["practice"] {
		t.Fatalf("migration did not add status/practice columns: %+v", columns)
	}
}

func TestValidProblemStatus(t *testing.T) {
	valid := []string{"pending", "open", "done", "tests_passed", "accepted", "practice_1", "practice_9"}
	for _, status := range valid {
		if !validProblemStatus(status) {
			t.Errorf("validProblemStatus(%q) = false", status)
		}
	}
	invalid := []string{"", "practice_0", "practice_10", "practice_x", "unknown"}
	for _, status := range invalid {
		if validProblemStatus(status) {
			t.Errorf("validProblemStatus(%q) = true", status)
		}
	}
}

func TestDeriveProblemState(t *testing.T) {
	cases := []struct {
		name     string
		events   []timeEvent
		status   string
		practice int
	}{
		{"empty", nil, problemStatusPending, 0},
		{"open", []timeEvent{{Type: TimeEventStart}}, problemStatusOpen, 0},
		{"done", []timeEvent{{Type: TimeEventStart}, {Type: TimeEventEnd}}, problemStatusDone, 0},
		{"accepted", []timeEvent{{Type: TimeEventStart}, {Type: TimeEventEnd}, {Type: TimeEventSubmit, Verdict: "OK"}}, problemStatusAccepted, 0},
		{"no practice count outside practice", []timeEvent{
			{Type: TimeEventSubmit, Verdict: "OK"}, {Type: TimeEventSubmit, Verdict: "OK"},
		}, problemStatusAccepted, 0},
		{"first accepted in practice", []timeEvent{
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
		}, problemStatusAccepted, 0},
		{"practice 1", []timeEvent{
			{Type: TimeEventSubmit, Verdict: "OK"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
		}, practiceStatus(1), 1},
		{"practice 2", []timeEvent{
			{Type: TimeEventSubmit, Verdict: "OK"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
		}, practiceStatus(2), 2},
		{"practice cap", []timeEvent{
			{Type: TimeEventSubmit, Verdict: "OK"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
			{Type: TimeEventSubmit, Verdict: "OK", Practice: "drill"},
		}, "practice_9", 9},
		{"wrong answers ignored", []timeEvent{{Type: TimeEventSubmit, Verdict: "OK"}, {Type: TimeEventSubmit, Verdict: "WRONG_ANSWER"}}, problemStatusAccepted, 0},
		{"tests", []timeEvent{{Type: TimeEventStart}, {Type: TimeEventTest}}, problemStatusTestsPassed, 0},
		{"tests after end", []timeEvent{{Type: TimeEventStart}, {Type: TimeEventEnd}, {Type: TimeEventTest}}, problemStatusTestsPassed, 0},
		{"accepted wins", []timeEvent{{Type: TimeEventTest}, {Type: TimeEventSubmit, Verdict: "OK"}}, problemStatusAccepted, 0},
		{"reopened", []timeEvent{{Type: TimeEventStart}, {Type: TimeEventEnd}, {Type: TimeEventStart}}, problemStatusOpen, 0},
	}
	for _, c := range cases {
		status, practice := deriveProblemState(c.events)
		if status != c.status || practice != c.practice {
			t.Errorf("%v: got (%q, %v), want (%q, %v)", c.name, status, practice, c.status, c.practice)
		}
	}
}

func TestVerdictFromNote(t *testing.T) {
	cases := map[string]string{
		"verdict OK submission 123": "OK",
		"verdict WRONG_ANSWER":      "WRONG_ANSWER",
		"all samples passed":        "",
		"":                          "",
	}
	for note, want := range cases {
		if got := verdictFromNote(note); got != want {
			t.Errorf("verdictFromNote(%q) = %q, want %q", note, got, want)
		}
	}
}
