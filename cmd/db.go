package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/fatih/color"
	"github.com/mitchellh/go-homedir"
	_ "modernc.org/sqlite"
)

const timerDBVersion = 1

const timerDBSchema = `
CREATE TABLE IF NOT EXISTS contest (
	key         TEXT PRIMARY KEY,
	type        TEXT NOT NULL DEFAULT '',
	contest_id  TEXT NOT NULL DEFAULT '',
	group_id    TEXT NOT NULL DEFAULT '',
	name        TEXT NOT NULL DEFAULT '',
	div         TEXT NOT NULL DEFAULT '',
	start_at    INTEGER NOT NULL DEFAULT 0,
	end_at      INTEGER NOT NULL DEFAULT 0,
	link        TEXT NOT NULL DEFAULT '',
	updated_at  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS problem (
	key         TEXT PRIMARY KEY,
	contest_key TEXT NOT NULL DEFAULT '',
	problem_id  TEXT NOT NULL DEFAULT '',
	display     TEXT NOT NULL DEFAULT '',
	name        TEXT NOT NULL DEFAULT '',
	link        TEXT NOT NULL DEFAULT '',
	dir         TEXT NOT NULL DEFAULT '',
	summary     TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT 'pending',
	practice    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS event (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	problem_key TEXT NOT NULL,
	type        TEXT NOT NULL,
	at_ms       INTEGER NOT NULL DEFAULT 0,
	elapsed_ms  INTEGER NOT NULL DEFAULT 0,
	duration_ms INTEGER NOT NULL DEFAULT 0,
	verdict     TEXT NOT NULL DEFAULT '',
	note        TEXT NOT NULL DEFAULT '',
	UNIQUE(problem_key, type, at_ms, note)
);

CREATE INDEX IF NOT EXISTS idx_event_problem ON event(problem_key, at_ms);
CREATE INDEX IF NOT EXISTS idx_event_type_at ON event(type, at_ms);
CREATE INDEX IF NOT EXISTS idx_problem_contest ON problem(contest_key);

CREATE VIEW IF NOT EXISTS problem_time AS
SELECT problem_key,
       SUM(duration_ms) AS total_ms,
       COUNT(*) AS event_count,
       MIN(CASE WHEN type = 'start' THEN at_ms END) AS first_start_ms,
       MAX(CASE WHEN type = 'end' THEN at_ms END) AS last_end_ms
FROM event
GROUP BY problem_key;

CREATE VIEW IF NOT EXISTS contest_time AS
SELECT p.contest_key,
       SUM(e.duration_ms) AS total_ms,
       COUNT(DISTINCT e.problem_key) AS problem_count
FROM event e
JOIN problem p ON p.key = e.problem_key
GROUP BY p.contest_key;
`

// timerDBPath returns the singleton timer database path.
func timerDBPath() string {
	if path := strings.TrimSpace(os.Getenv("CF_TIMER_DB")); path != "" {
		return path
	}
	home, err := homedir.Dir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".cf", "timer.db")
}

func openTimerDB(readOnly bool) (*sql.DB, error) {
	path := timerDBPath()
	if readOnly {
		if _, err := os.Stat(path); err != nil {
			return nil, fmt.Errorf("No timer database at %v. Run `cf db sync` first", path)
		}
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)"
	if readOnly {
		dsn = "file:" + path + "?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)"
	} else {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, err
		}
		dsn += "&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if !readOnly {
		if err = initTimerDBSchema(db); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func initTimerDBSchema(db *sql.DB) error {
	if _, err := db.Exec(timerDBSchema); err != nil {
		return err
	}
	if err := ensureProblemColumns(db); err != nil {
		return err
	}
	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %v", timerDBVersion))
	return err
}

// ensureProblemColumns migrates databases created before a column was added.
func ensureProblemColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(problem)`)
	if err != nil {
		return err
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue interface{}
		if err = rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	if !columns["status"] {
		if _, err = db.Exec(`ALTER TABLE problem ADD COLUMN status TEXT NOT NULL DEFAULT 'pending'`); err != nil {
			return err
		}
	}
	if columns["tests_passed"] {
		// The state is folded into status now; the old column is redundant.
		db.Exec(`ALTER TABLE problem DROP COLUMN tests_passed`)
	}
	if !columns["practice"] {
		if _, err = db.Exec(`ALTER TABLE problem ADD COLUMN practice INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	return nil
}

// timerKeys is the canonical identity of a contest/problem in the database.
type timerKeys struct {
	ContestKey string
	ProblemKey string
	ProblemID  string
	Type       string
	ContestID  string
	GroupID    string
}

func normalizeProblemType(problemType, contestID string) string {
	if problemType == "" || problemType == "contest" {
		if len(contestID) < 6 {
			return "contest"
		}
		return "gym"
	}
	return problemType
}

func formatContestKey(problemType, contestID, groupID string) string {
	switch problemType {
	case "group":
		if groupID != "" && contestID != "" {
			return "group:" + groupID + ":" + contestID
		}
	case "gym":
		if contestID != "" {
			return "gym:" + contestID
		}
	case "acmsguru":
		if contestID != "" {
			return "acmsguru:" + contestID
		}
	}
	if contestID != "" {
		return "contest:" + contestID
	}
	return ""
}

func keysFromInfo(info client.Info) timerKeys {
	problemType := normalizeProblemType(info.ProblemType, info.ContestID)
	contestKey := formatContestKey(problemType, info.ContestID, info.GroupID)
	keys := timerKeys{
		ContestKey: contestKey,
		ProblemID:  strings.ToLower(info.ProblemID),
		Type:       problemType,
		ContestID:  info.ContestID,
		GroupID:    info.GroupID,
	}
	if contestKey != "" && keys.ProblemID != "" {
		keys.ProblemKey = contestKey + ":" + keys.ProblemID
	}
	return keys
}

// keysFromRecord derives database keys from the record link or its directory.
func keysFromRecord(rec *problemTimeRecord, dir string) timerKeys {
	parsed := map[string]string{}
	if rec.Link != "" {
		parsed = parseArg(rec.Link)
	}
	if parsed["contestID"] == "" {
		if config.Instance != nil {
			for k, v := range parsePath(dir) {
				if parsed[k] == "" {
					parsed[k] = v
				}
			}
		}
	}
	info := client.Info{
		ProblemType: parsed["problemType"],
		ContestID:   parsed["contestID"],
		GroupID:     parsed["groupID"],
		ProblemID:   parsed["problemID"],
	}
	if info.ProblemID == "" {
		info.ProblemID = filepath.Base(dir)
	}
	keys := keysFromInfo(info)
	if keys.ContestKey == "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			abs = dir
		}
		keys.ContestKey = "local:" + filepath.ToSlash(filepath.Dir(abs))
		keys.ProblemID = strings.ToLower(filepath.Base(dir))
		keys.ProblemKey = keys.ContestKey + ":" + keys.ProblemID
	}
	return keys
}

func contestLinkFor(keys timerKeys) string {
	host := "https://codeforces.com"
	if config.Instance != nil && config.Instance.Host != "" {
		host = config.Instance.Host
	}
	info := client.Info{
		ProblemType: keys.Type,
		ContestID:   keys.ContestID,
		GroupID:     keys.GroupID,
	}
	if URL, err := info.ProblemSetURL(host); err == nil {
		return URL
	}
	return ""
}

func parseEventTime(raw string) time.Time {
	t, err := time.Parse(timeLayout, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseElapsedString(raw string) int64 {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 3 {
		return 0
	}
	var h, m, s int64
	if _, err := fmt.Sscanf(parts[0], "%d", &h); err != nil {
		return 0
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
		return 0
	}
	if _, err := fmt.Sscanf(parts[2], "%d", &s); err != nil {
		return 0
	}
	return ((h*60+m)*60 + s) * 1000
}

func eventElapsedMS(ev timeEvent) int64 {
	if ev.ElapsedMS > 0 {
		return ev.ElapsedMS
	}
	return parseElapsedString(ev.Elapsed)
}

// Problem completion states used by the database and query commands.
const (
	problemStatusPending     = "pending"
	problemStatusOpen        = "open"
	problemStatusDone        = "done"
	problemStatusTestsPassed = "tests_passed"
	problemStatusAccepted    = "accepted"
	problemStatusPractice    = "practice_"
	problemStatusMaxPractice = 9
)

// practiceStatus returns practice_N for N in 1..9.
func practiceStatus(n int) string {
	if n > problemStatusMaxPractice {
		n = problemStatusMaxPractice
	}
	if n < 1 {
		n = 1
	}
	return fmt.Sprintf("%v%v", problemStatusPractice, n)
}

func validProblemStatus(status string) bool {
	switch status {
	case problemStatusPending, problemStatusOpen, problemStatusDone, problemStatusTestsPassed, problemStatusAccepted:
		return true
	}
	if strings.HasPrefix(status, problemStatusPractice) {
		n, err := strconv.Atoi(strings.TrimPrefix(status, problemStatusPractice))
		return err == nil && n >= 1 && n <= problemStatusMaxPractice
	}
	return false
}

// deriveProblemState reduces one record's events to a completion label and the
// capped number of accepted submissions made in practice mode.
func deriveProblemState(events []timeEvent) (status string, practice int) {
	open, done, testsPassed := false, false, false
	accepted := 0
	for _, ev := range events {
		switch ev.Type {
		case TimeEventStart:
			open = true
		case TimeEventEnd:
			open = false
			done = true
		case TimeEventTest:
			testsPassed = true
		case TimeEventSubmit:
			verdict := ev.Verdict
			if verdict == "" {
				verdict = verdictFromNote(ev.Note)
			}
			if verdict == "OK" {
				accepted++
				// The first accepted submit establishes "accepted"; only later
				// accepted submits made in practice mode count as practice runs.
				if accepted > 1 && ev.Practice != "" {
					practice++
				}
			}
		}
	}
	if practice > problemStatusMaxPractice {
		practice = problemStatusMaxPractice
	}
	switch {
	case practice > 0:
		status = practiceStatus(practice)
	case accepted > 0:
		status = problemStatusAccepted
	case testsPassed:
		status = problemStatusTestsPassed
	case open:
		status = problemStatusOpen
	case done:
		status = problemStatusDone
	default:
		status = problemStatusPending
	}
	return
}

func verdictFromNote(note string) string {
	note = strings.TrimSpace(note)
	if !strings.HasPrefix(note, "verdict ") {
		return ""
	}
	fields := strings.Fields(strings.TrimPrefix(note, "verdict "))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// indexProblemRecord reads one count_time.json and upserts it into the timer DB.
func indexProblemRecord(dir string) error {
	rec, err := loadProblemTimeRecord(filepath.Join(dir, TimeRecordFileName))
	if err != nil {
		return err
	}
	db, err := openTimerDB(false)
	if err != nil {
		return err
	}
	defer db.Close()
	return indexProblemRecordDB(db, dir, rec)
}

func indexProblemRecordQuiet(dir string) {
	if err := indexProblemRecord(dir); err != nil {
		fmt.Fprintln(os.Stderr, color.YellowString("Timer DB: %v", err))
	}
}

func indexProblemRecordDB(db *sql.DB, dir string, rec *problemTimeRecord) error {
	keys := keysFromRecord(rec, dir)
	display := rec.Problem
	if display == "" {
		display = keys.ProblemID
	}
	status, practice := deriveProblemState(rec.Events)
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	if _, err = tx.Exec(`INSERT INTO contest(key, type, contest_id, group_id, link, updated_at)
		VALUES(?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			type = excluded.type,
			contest_id = excluded.contest_id,
			group_id = excluded.group_id,
			link = CASE WHEN excluded.link <> '' THEN excluded.link ELSE contest.link END,
			updated_at = excluded.updated_at`,
		keys.ContestKey, keys.Type, keys.ContestID, keys.GroupID, contestLinkFor(keys), now); err != nil {
		return err
	}

	if _, err = tx.Exec(`INSERT INTO problem(key, contest_key, problem_id, display, link, dir, summary, status, practice)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			contest_key = excluded.contest_key,
			problem_id = excluded.problem_id,
			display = CASE WHEN excluded.display <> '' THEN excluded.display ELSE problem.display END,
			link = CASE WHEN excluded.link <> '' THEN excluded.link ELSE problem.link END,
			dir = CASE WHEN excluded.dir <> '' THEN excluded.dir ELSE problem.dir END,
			summary = CASE WHEN excluded.summary <> '' THEN excluded.summary ELSE problem.summary END,
			status = excluded.status,
			practice = excluded.practice`,
		keys.ProblemKey, keys.ContestKey, keys.ProblemID, display, rec.Link, dir, rec.Summary, status, practice); err != nil {
		return err
	}

	var openStart int64 = -1
	for _, ev := range rec.Events {
		elapsed := eventElapsedMS(ev)
		duration := ev.DurationMS
		if ev.Type == TimeEventStart {
			openStart = elapsed
		}
		if ev.Type == TimeEventEnd {
			if duration == 0 && openStart >= 0 && elapsed >= openStart {
				duration = elapsed - openStart
			}
			openStart = -1
		}
		if duration < 0 {
			duration = 0
		}
		at := parseEventTime(ev.Time)
		var atMS int64
		if !at.IsZero() {
			atMS = at.UnixMilli()
		}
		verdict := ev.Verdict
		if verdict == "" && ev.Type == TimeEventSubmit {
			verdict = verdictFromNote(ev.Note)
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO event(problem_key, type, at_ms, elapsed_ms, duration_ms, verdict, note)
			VALUES(?, ?, ?, ?, ?, ?, ?)`,
			keys.ProblemKey, ev.Type, atMS, elapsed, duration, verdict, ev.Note); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// collectRecordFiles finds every count_time.json reachable from the timer
// state, the current folder tree, and the given specifiers.
func collectRecordFiles(specifiers []string) []string {
	set := map[string]bool{}
	if st, err := loadTimeState(); err == nil {
		for _, path := range st.Records {
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				set[filepath.Clean(path)] = true
			}
		}
	}
	if root := findCurrentRoot(); root != "" {
		addRecordFiles(set, root)
	}
	for _, specifier := range specifiers {
		info, err := resolveTimerInfo(specifier)
		if err != nil {
			continue
		}
		addRecordFiles(set, info.Path())
	}
	files := make([]string, 0, len(set))
	for path := range set {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func addRecordFiles(set map[string]bool, path string) {
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	if !info.IsDir() {
		if filepath.Base(path) == TimeRecordFileName {
			set[filepath.Clean(path)] = true
		}
		return
	}
	filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi == nil || fi.IsDir() {
			return nil
		}
		if fi.Name() == TimeRecordFileName {
			set[filepath.Clean(p)] = true
		}
		return nil
	})
}

func findCurrentRoot() string {
	rootName := "cf"
	if config.Instance != nil && config.Instance.FolderName["root"] != "" {
		rootName = config.Instance.FolderName["root"]
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	path := cwd
	for {
		if filepath.Base(path) == rootName {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			break
		}
		path = parent
	}
	candidate := filepath.Join(cwd, rootName)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}

// dbSync imports every reachable record file and fills missing contest metadata.
func dbSync(specifiers []string, refresh bool) error {
	for _, specifier := range specifiers {
		if _, err := resolveTimerInfo(specifier); err != nil {
			return err
		}
	}
	db, err := openTimerDB(false)
	if err != nil {
		return err
	}
	defer db.Close()

	files := collectRecordFiles(specifiers)
	indexed := 0
	for _, path := range files {
		rec, err := loadProblemTimeRecord(path)
		if err != nil {
			continue
		}
		if err = indexProblemRecordDB(db, filepath.Dir(path), rec); err != nil {
			return err
		}
		indexed++
	}
	color.Green("Indexed %v record file(s) into %v.", indexed, timerDBPath())

	fetched, failed := refreshContestMeta(db, refresh)
	if fetched > 0 || failed > 0 {
		color.Green("Contest metadata: %v fetched, %v unavailable.", fetched, failed)
	}
	return nil
}

func refreshContestMeta(db *sql.DB, refresh bool) (fetched, failed int) {
	if client.Instance == nil {
		return 0, 0
	}
	rows, err := db.Query(`SELECT key, type, contest_id, group_id, name, start_at FROM contest ORDER BY key`)
	if err != nil {
		return 0, 0
	}
	type contestRow struct {
		key, problemType, contestID, groupID, name string
		startAt                                    int64
	}
	var contests []contestRow
	for rows.Next() {
		var row contestRow
		if err = rows.Scan(&row.key, &row.problemType, &row.contestID, &row.groupID, &row.name, &row.startAt); err != nil {
			continue
		}
		if row.problemType == "acmsguru" || row.contestID == "" {
			continue
		}
		if !refresh && row.name != "" && row.startAt != 0 {
			continue
		}
		contests = append(contests, row)
	}
	rows.Close()

	for i, row := range contests {
		if i > 0 {
			time.Sleep(600 * time.Millisecond)
		}
		meta, err := client.Instance.ContestMeta(row.contestID)
		if err != nil {
			failed++
			continue
		}
		endAt := meta.StartTimeSeconds + meta.DurationSeconds
		if _, err = db.Exec(`UPDATE contest SET name = ?, div = ?, start_at = ?, end_at = ?, updated_at = ? WHERE key = ?`,
			meta.Name, meta.Div, meta.StartTimeSeconds, endAt, time.Now().Unix(), row.key); err != nil {
			failed++
			continue
		}
		for _, problem := range meta.Problems {
			problemID := strings.ToLower(problem.Index)
			problemKey := row.key + ":" + problemID
			link := ""
			info := client.Info{ProblemType: row.problemType, ContestID: row.contestID, GroupID: row.groupID, ProblemID: problem.Index}
			host := "https://codeforces.com"
			if config.Instance != nil && config.Instance.Host != "" {
				host = config.Instance.Host
			}
			if URL, err := info.ProblemURL(host); err == nil {
				link = URL
			}
			db.Exec(`INSERT INTO problem(key, contest_key, problem_id, name, link)
				VALUES(?, ?, ?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET
					name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE problem.name END,
					link = CASE WHEN excluded.link <> '' THEN excluded.link ELSE problem.link END`,
				problemKey, row.key, problemID, problem.Name, link)
		}
		fetched++
	}
	return fetched, failed
}
