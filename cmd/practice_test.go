package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
)

func TestParseDifficultyRange(t *testing.T) {
	cases := []struct {
		raw      string
		min, max int
		wantErr  bool
	}{
		{"", 0, 0, false},
		{"1200", 1200, 1200, false},
		{"800-1200", 800, 1200, false},
		{"800..1200", 800, 1200, false},
		{" 900 ", 900, 900, false},
		{"1200-800", 0, 0, true},
		{"abc", 0, 0, true},
		{"1-2-3", 0, 0, true},
		{"-1", 0, 0, true},
	}
	for _, c := range cases {
		minRating, maxRating, err := parseDifficultyRange(c.raw)
		if (err != nil) != c.wantErr || minRating != c.min || maxRating != c.max {
			t.Errorf("parseDifficultyRange(%q) = (%v, %v, %v), want (%v, %v, err=%v)",
				c.raw, minRating, maxRating, err, c.min, c.max, c.wantErr)
		}
	}
}

func TestParsePracticeTags(t *testing.T) {
	got := parsePracticeTags("dp, greedy  DP, math")
	want := []string{"dp", "greedy", "math"}
	if len(got) != len(want) {
		t.Fatalf("tags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("tags = %v, want %v", got, want)
		}
	}
}

func TestFilterOnlineProblems(t *testing.T) {
	all := []client.ProblemsetProblem{
		{ContestID: 1, Index: "A", Rating: 800, Tags: []string{"implementation"}},
		{ContestID: 2, Index: "B", Rating: 1500, Tags: []string{"dp"}},
		{ContestID: 3, Index: "C", Rating: 1200, Tags: []string{"greedy", "math"}},
		{ContestID: 0, Index: "D", Rating: 800},
	}
	got := filterOnlineProblems(all, 800, 1500, []string{"dp", "math"})
	if len(got) != 2 || got[0].ContestID != 2 || got[1].ContestID != 3 {
		t.Fatalf("filterOnlineProblems = %+v", got)
	}
	if got := filterOnlineProblems(all, 0, 0, nil); len(got) != 3 {
		t.Fatalf("unfiltered count = %v, want 3", len(got))
	}
}

func TestSanitizePracticeName(t *testing.T) {
	cases := map[string]string{
		"":             "practice",
		"drill":        "drill",
		"../evil name": "evil_name",
		"a/b":          "a_b",
		"练习 1":         "练习_1",
	}
	for in, want := range cases {
		if got := sanitizePracticeName(in); got != want {
			t.Errorf("sanitizePracticeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectLocalPractice(t *testing.T) {
	t.Setenv("CF_TIMER_DB", filepath.Join(t.TempDir(), "timer.db"))
	db, err := openTimerDB(false)
	if err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		key, display, status string
	}{
		{"contest:1136:a", "1136a", problemStatusAccepted},
		{"contest:1136:b", "1136b", problemStatusDone},
		{"contest:1136:c", "1136c", problemStatusPending},
	}
	for _, row := range rows {
		if _, err = db.Exec(`INSERT INTO problem(key, contest_key, problem_id, display, status) VALUES(?, ?, ?, ?, ?)`,
			row.key, "contest:1136", row.key[len(row.key)-1:], row.display, row.status); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	all, err := selectLocalPractice("", 10)
	if err != nil || len(all) != 3 {
		t.Fatalf("select all = %v, %v", len(all), err)
	}
	accepted, err := selectLocalPractice(problemStatusAccepted, 10)
	if err != nil || len(accepted) != 1 || accepted[0].Key != "contest:1136:a" {
		t.Fatalf("select accepted = %+v, %v", accepted, err)
	}
	pending, err := selectLocalPractice(problemStatusPending, 10)
	if err != nil || len(pending) != 1 || pending[0].Key != "contest:1136:c" {
		t.Fatalf("select pending = %+v, %v", pending, err)
	}
	if accepted[0].Link == "" {
		t.Fatal("selected problem should have a link")
	}
}

// initTestConfig installs a temporary empty configuration and restores the
// previous global instance after the test.
func initTestConfig(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	old := config.Instance
	t.Cleanup(func() { config.Instance = old })
	config.Init(path)
}

func TestResolvePracticeTimer(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	initTestConfig(t)

	base := filepath.Join(work, "cf", "practice", "drill")
	problemDir := filepath.Join(base, "1136a")
	if err := os.MkdirAll(problemDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := practiceManifest{
		Name:     "drill",
		Problems: []practiceProblem{{Key: "contest:1136:a", Display: "1136a"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(base, "practice.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	info, dir, problem, err := resolvePracticeTimer("p_drill", "1136a")
	if err != nil {
		t.Fatal(err)
	}
	if info.ContestID != "1136" || dir != problemDir || problem != "1136a" {
		t.Fatalf("resolvePracticeTimer = (%+v, %q, %q)", info, dir, problem)
	}
	if _, _, _, err = resolvePracticeTimer("p_drill", "1136b"); err == nil {
		t.Fatal("problem outside the practice set must be rejected")
	}
	if _, _, _, err = resolvePracticeTimer("p_missing", "1136a"); err == nil {
		t.Fatal("missing practice set must be rejected")
	}

	// Manifest miss falls back to <contestId><problemIndex>.
	fallbackDir := filepath.Join(base, "1136b")
	if err = os.MkdirAll(fallbackDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, dir, problem, err = resolvePracticeTimer("p_drill", "1136b"); err != nil || dir != fallbackDir || problem != "1136b" {
		t.Fatalf("fallback resolve = (%q, %q, %v)", dir, problem, err)
	}

	// A practice set without practice.json still resolves by folder name.
	noManifestBase := filepath.Join(work, "cf", "practice", "plain")
	noManifestDir := filepath.Join(noManifestBase, "1136c")
	if err = os.MkdirAll(noManifestDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, dir, _, err = resolvePracticeTimer("p_plain", "1136c"); err != nil || dir != noManifestDir {
		t.Fatalf("manifest-less resolve = (%q, %v)", dir, err)
	}
}

func TestTimerPracticeFlow(t *testing.T) {
	work := t.TempDir()
	t.Chdir(work)
	initTestConfig(t)
	oldClient := client.Instance
	client.Instance = nil
	defer func() { client.Instance = oldClient }()
	oldArgs := Args
	Args = &ParsedArgs{}
	defer func() { Args = oldArgs }()
	t.Setenv("CF_TIMER_STATE", filepath.Join(work, "timer.json"))
	t.Setenv("CF_TIMER_DB", filepath.Join(work, "timer.db"))

	base := filepath.Join(work, "cf", "practice", "drill")
	problemDir := filepath.Join(base, "1136a")
	if err := os.MkdirAll(problemDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := practiceManifest{
		Name:     "drill",
		Problems: []practiceProblem{{Key: "contest:1136:a", Display: "1136a"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(base, "practice.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	if err = timerStart("p_drill", "1136a", "practice"); err != nil {
		t.Fatal(err)
	}
	t.Chdir(problemDir)
	if !appendAutoTimeEvent(TimeEventSubmit, "verdict OK submission 1") {
		t.Fatal("first submit was not recorded")
	}
	if !appendAutoTimeEvent(TimeEventSubmit, "verdict OK submission 2") {
		t.Fatal("second submit was not recorded")
	}
	if err = timerEnd("p_drill", "1136a", "done"); err != nil {
		t.Fatal(err)
	}

	rec, err := loadProblemTimeRecord(filepath.Join(problemDir, TimeRecordFileName))
	if err != nil {
		t.Fatal(err)
	}
	submits := 0
	for _, ev := range rec.Events {
		if ev.Type == TimeEventSubmit {
			submits++
			if ev.Practice != "drill" || ev.Verdict != "OK" {
				t.Fatalf("unexpected submit event: %+v", ev)
			}
		}
	}
	if submits != 2 {
		t.Fatalf("submit events = %v, want 2: %+v", submits, rec.Events)
	}
	if rec.Events[0].Type != TimeEventStart || rec.Events[0].Practice != "drill" {
		t.Fatalf("start event missing practice: %+v", rec.Events[0])
	}

	db, err := openTimerDB(true)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var status string
	var practice int
	if err = db.QueryRow(`SELECT status, practice FROM problem WHERE key = 'contest:1136:a'`).Scan(&status, &practice); err != nil {
		t.Fatal(err)
	}
	if status != practiceStatus(1) || practice != 1 {
		t.Fatalf("db state = (%q, %v), want (practice_1, 1)", status, practice)
	}
}

func TestParsePracticeKey(t *testing.T) {
	cases := map[string]string{
		"contest:1136:a":            "contest",
		"gym:100001:a":              "gym",
		"group:Cw4JRyRGXR:269760:a": "group",
		"acmsguru:99999:100":        "acmsguru",
	}
	for key, wantType := range cases {
		info, ok := parsePracticeKey(key)
		if !ok || info.ProblemType != wantType {
			t.Errorf("parsePracticeKey(%q) = (%+v, %v), want type %q", key, info, ok, wantType)
		}
	}
	if _, ok := parsePracticeKey("local:/tmp/x:a"); ok {
		t.Fatal("local keys should be rejected")
	}
}
