package cmd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// DB command
func DB() (err error) {
	args := Args.CommandArgs
	if len(args) == 0 {
		printDBHelp()
		return nil
	}
	switch strings.ToLower(args[0]) {
	case "sql", "query":
		return dbSQLCommand(args[1:], Args.DBJSON, Args.WriteDB)
	case "time", "show":
		return dbTimeCommand(args[1:], Args.DBJSON)
	case "stats", "stat":
		return dbStatsCommand(args[1:], Args.FromDate, Args.ToDate, Args.StatusDB, Args.AcceptedOnly, Args.DBJSON)
	case "sync":
		return dbSyncCommand(args[1:], Args.RefreshDB)
	case "path":
		fmt.Println(timerDBPath())
		return nil
	case "help", "-h", "--help":
		printDBHelp()
		return nil
	}
	return fmt.Errorf("Unknown db command %q. Use sql, time, stats, sync or path", args[0])
}

func printDBHelp() {
	fmt.Println(`Usage:
  cf db sql [-j] [--write] <SQL>
  cf db time [-j] [<specifier>...]
  cf db stats [-j] [--from <date>] [--to <date>] [--accepted] [<specifier>...]
  cf db sync [--refresh] [<specifier>...]
  cf db path

The timer database is a local index of the count_time.json files.
Default path: ~/.cf/timer.db (override with CF_TIMER_DB).`)
}

func dbSQLCommand(args []string, jsonOut, write bool) error {
	query := strings.TrimSpace(strings.Join(args, " "))
	if query == "" {
		return errors.New("Usage: cf db sql [-j] [--write] <SQL>")
	}
	db, err := openTimerDB(!write)
	if err != nil {
		return err
	}
	defer db.Close()

	if write && !isReadSQL(query) {
		result, err := db.Exec(query)
		if err != nil {
			return err
		}
		affected, _ := result.RowsAffected()
		fmt.Printf("OK, %v row(s) affected.\n", affected)
		return nil
	}

	rows, err := db.Query(query)
	if err != nil {
		return err
	}
	defer rows.Close()
	return printSQLRows(rows, jsonOut)
}

func isReadSQL(query string) bool {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return true
	}
	switch strings.ToLower(fields[0]) {
	case "select", "with", "pragma", "explain", "values":
		return true
	}
	return false
}

func printSQLRows(rows *sql.Rows, jsonOut bool) error {
	columns, err := rows.Columns()
	if err != nil {
		return err
	}
	var records []map[string]interface{}
	var tableRows [][]string
	for rows.Next() {
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return err
		}
		record := make(map[string]interface{}, len(columns))
		row := make([]string, len(columns))
		for i, value := range values {
			switch v := value.(type) {
			case nil:
				record[columns[i]] = nil
				row[i] = ""
			case []byte:
				record[columns[i]] = string(v)
				row[i] = string(v)
			case int64:
				record[columns[i]] = v
				row[i] = fmt.Sprint(v)
			case float64:
				record[columns[i]] = v
				row[i] = fmt.Sprint(v)
			default:
				record[columns[i]] = v
				row[i] = fmt.Sprint(v)
			}
		}
		records = append(records, record)
		tableRows = append(tableRows, row)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if jsonOut {
		return printDBJSON(records)
	}
	if len(tableRows) == 0 {
		fmt.Println("No rows.")
		return nil
	}
	table := tablewriter.NewWriter(color.Output)
	table.SetHeader(columns)
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for _, row := range tableRows {
		table.Append(row)
	}
	table.Render()
	return nil
}

func printDBJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}

type dbEvent struct {
	Type       string `json:"type"`
	AtMS       int64  `json:"at_ms"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	DurationMS int64  `json:"duration_ms"`
	Verdict    string `json:"verdict,omitempty"`
	Note       string `json:"note,omitempty"`
}

type dbProblemTime struct {
	Key      string    `json:"key"`
	Display  string    `json:"display"`
	Name     string    `json:"name,omitempty"`
	Link     string    `json:"link,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	Status   string    `json:"status"`
	Practice int       `json:"practice"`
	TotalMS  int64     `json:"total_ms"`
	Events   []dbEvent `json:"events"`
}

type dbContestTime struct {
	Key      string          `json:"key"`
	Name     string          `json:"name,omitempty"`
	Div      string          `json:"div,omitempty"`
	StartAt  int64           `json:"start_at,omitempty"`
	EndAt    int64           `json:"end_at,omitempty"`
	Link     string          `json:"link,omitempty"`
	TotalMS  int64           `json:"total_ms"`
	SpanMS   int64           `json:"span_ms"`
	Problems []dbProblemTime `json:"problems"`
}

func formatUnixMS(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).Format("2006-01-02 15:04:05")
}

func dbTimeCommand(rest []string, jsonOut bool) error {
	var info client.Info
	var err error
	cwd, e := os.Getwd()
	if e != nil {
		return e
	}
	if len(rest) == 0 {
		info, err = infoFromSpecifiers(nil, cwd)
	} else {
		info, err = infoFromSpecifiers(rest, cwd)
	}
	if err == nil && info.ContestID == "" {
		err = errors.New("No current contest/problem. Use `cf db time <specifier>`")
	}
	if err != nil {
		return err
	}
	keys := keysFromInfo(info)
	if keys.ContestKey == "" {
		return fmt.Errorf("Cannot understand the specifier")
	}
	db, err := openTimerDB(true)
	if err != nil {
		return err
	}
	defer db.Close()

	if keys.ProblemKey != "" {
		result, err := loadDBProblemTime(db, keys.ProblemKey, timerLink(&info))
		if err != nil {
			return err
		}
		if jsonOut {
			return printDBJSON(result)
		}
		printDBProblemTime(result)
		return nil
	}

	result, err := loadDBContestTime(db, keys.ContestKey)
	if err != nil {
		return err
	}
	if jsonOut {
		return printDBJSON(result)
	}
	printDBContestTime(result)
	return nil
}

func loadDBProblemTime(db *sql.DB, key, link string) (*dbProblemTime, error) {
	row := db.QueryRow(`SELECT key, display, name, link, summary, status, practice FROM problem WHERE key = ? OR (link <> '' AND link = ?) LIMIT 1`, key, link)
	result := &dbProblemTime{}
	if err := row.Scan(&result.Key, &result.Display, &result.Name, &result.Link, &result.Summary, &result.Status, &result.Practice); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("No timer data for %v. Run `cf db sync` first", key)
		}
		return nil, err
	}
	rows, err := db.Query(`SELECT type, at_ms, elapsed_ms, duration_ms, verdict, note FROM event WHERE problem_key = ? ORDER BY at_ms, id`, result.Key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var ev dbEvent
		if err = rows.Scan(&ev.Type, &ev.AtMS, &ev.ElapsedMS, &ev.DurationMS, &ev.Verdict, &ev.Note); err != nil {
			return nil, err
		}
		result.Events = append(result.Events, ev)
		result.TotalMS += ev.DurationMS
	}
	return result, rows.Err()
}

func loadDBContestTime(db *sql.DB, key string) (*dbContestTime, error) {
	row := db.QueryRow(`SELECT key, name, div, start_at, end_at, link FROM contest WHERE key = ?`, key)
	result := &dbContestTime{}
	if err := row.Scan(&result.Key, &result.Name, &result.Div, &result.StartAt, &result.EndAt, &result.Link); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("No timer data for %v. Run `cf db sync` first", key)
		}
		return nil, err
	}
	rows, err := db.Query(`SELECT p.key, COALESCE(SUM(e.duration_ms), 0)
		FROM problem p JOIN event e ON e.problem_key = p.key
		WHERE p.contest_key = ?
		GROUP BY p.key ORDER BY p.key`, key)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var problemKey string
		var total int64
		if err = rows.Scan(&problemKey, &total); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, problemKey)
		result.TotalMS += total
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, problemKey := range keys {
		problem, err := loadDBProblemTime(db, problemKey, "")
		if err != nil {
			return nil, err
		}
		result.Problems = append(result.Problems, *problem)
	}
	var firstStart, lastEnd sql.NullInt64
	db.QueryRow(`SELECT MIN(CASE WHEN e.type = 'start' THEN e.at_ms END),
		MAX(CASE WHEN e.type = 'end' THEN e.at_ms END)
		FROM event e JOIN problem p ON p.key = e.problem_key WHERE p.contest_key = ?`, key).Scan(&firstStart, &lastEnd)
	if firstStart.Valid && lastEnd.Valid && lastEnd.Int64 >= firstStart.Int64 {
		result.SpanMS = lastEnd.Int64 - firstStart.Int64
	}
	return result, nil
}

func printDBProblemTime(result *dbProblemTime) {
	title := result.Display
	if title == "" {
		title = result.Key
	}
	if result.Name != "" && result.Name != title {
		title += " — " + result.Name
	}
	title += " [" + result.Status + "]"
	color.Cyan("Problem %v", title)
	if result.Link != "" {
		fmt.Printf("Link: %v\n", result.Link)
	}
	if result.Summary != "" {
		fmt.Printf("Summary: %v\n", result.Summary)
	}
	if len(result.Events) == 0 {
		fmt.Println("No events.")
		return
	}
	table := tablewriter.NewWriter(color.Output)
	table.SetHeader([]string{"time", "elapsed", "duration", "type", "note"})
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for _, ev := range result.Events {
		duration := "-"
		if ev.DurationMS > 0 {
			duration = formatElapsedMS(ev.DurationMS)
		}
		note := ev.Note
		if ev.Verdict != "" {
			note = strings.TrimSpace(ev.Verdict + " " + note)
		}
		table.Append([]string{formatUnixMS(ev.AtMS), formatElapsedMS(ev.ElapsedMS), duration, ev.Type, note})
	}
	table.Render()
	color.Green("Total: %v", formatElapsedMS(result.TotalMS))
}

func printDBContestTime(result *dbContestTime) {
	title := result.Key
	if result.Name != "" {
		title = result.Name
	}
	if result.Div != "" && !strings.Contains(strings.ToLower(result.Name), "div") {
		title += " (Div. " + result.Div + ")"
	}
	color.Cyan("Contest %v", title)
	fmt.Printf("Link: %v\n", result.Link)
	if result.StartAt > 0 {
		fmt.Printf("Start: %v  End: %v\n", formatUnixMS(result.StartAt*1000), formatUnixMS(result.EndAt*1000))
	}
	if len(result.Problems) == 0 {
		fmt.Println("No events.")
		return
	}
	table := tablewriter.NewWriter(color.Output)
	table.SetHeader([]string{"problem", "name", "status", "time"})
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for _, problem := range result.Problems {
		table.Append([]string{problem.Display, problem.Name, problem.Status, formatElapsedMS(problem.TotalMS)})
	}
	table.Render()
	color.Green("Sum: %v   Span: %v", formatElapsedMS(result.TotalMS), formatElapsedMS(result.SpanMS))
}

type statsProblem struct {
	Key      string `json:"key"`
	Display  string `json:"display"`
	Name     string `json:"name,omitempty"`
	Contest  string `json:"contest"`
	Status   string `json:"status"`
	Practice int    `json:"practice"`
	TotalMS  int64  `json:"total_ms"`
	LastEnd  int64  `json:"last_end_ms"`
	Accepted bool   `json:"accepted"`
}

type statsResult struct {
	From      string         `json:"from"`
	To        string         `json:"to"`
	Accepted  bool           `json:"accepted_only"`
	Count     int            `json:"count"`
	AcceptedN int            `json:"accepted_count"`
	TotalMS   int64          `json:"total_ms"`
	Problems  []statsProblem `json:"problems"`
	PerDay    []statsDay     `json:"per_day"`
}

type statsDay struct {
	Day     string `json:"day"`
	Count   int    `json:"count"`
	TotalMS int64  `json:"total_ms"`
}

func dbStatsCommand(specifiers []string, fromRaw, toRaw, statusFilter string, acceptedOnly, jsonOut bool) error {
	statusFilter = strings.ToLower(strings.TrimSpace(statusFilter))
	if statusFilter != "" && !validProblemStatus(statusFilter) {
		return fmt.Errorf("Invalid --status %q. Use pending, open, done, tests_passed, accepted or practice_1..practice_9", statusFilter)
	}
	fromMS, toMS := int64(0), int64(1<<63-1)
	if fromRaw != "" {
		t, err := time.ParseInLocation("2006-01-02", fromRaw, time.Local)
		if err != nil {
			return fmt.Errorf("Invalid --from date %q, use YYYY-MM-DD", fromRaw)
		}
		fromMS = t.UnixMilli()
	}
	if toRaw != "" {
		t, err := time.ParseInLocation("2006-01-02", toRaw, time.Local)
		if err != nil {
			return fmt.Errorf("Invalid --to date %q, use YYYY-MM-DD", toRaw)
		}
		toMS = t.AddDate(0, 0, 1).UnixMilli()
	}

	query := `SELECT p.key, p.display, p.name, p.contest_key, p.status, p.practice,
		COALESCE(SUM(e.duration_ms), 0), MAX(e.at_ms),
		EXISTS(SELECT 1 FROM event s WHERE s.problem_key = p.key AND s.type = 'submit' AND s.verdict = 'OK')
		FROM event e JOIN problem p ON p.key = e.problem_key
		WHERE e.type = 'end' AND e.at_ms >= ? AND e.at_ms < ?`
	params := []interface{}{fromMS, toMS}
	problemKeys, contestKeys := dbStatsFilters(specifiers)
	if len(problemKeys) > 0 || len(contestKeys) > 0 {
		var parts []string
		if len(problemKeys) > 0 {
			parts = append(parts, "p.key IN ("+placeholders(len(problemKeys))+")")
			for _, key := range problemKeys {
				params = append(params, key)
			}
		}
		if len(contestKeys) > 0 {
			parts = append(parts, "p.contest_key IN ("+placeholders(len(contestKeys))+")")
			for _, key := range contestKeys {
				params = append(params, key)
			}
		}
		query += " AND (" + strings.Join(parts, " OR ") + ")"
	}
	if statusFilter != "" {
		query += " AND p.status = ?"
		params = append(params, statusFilter)
	}
	query += " GROUP BY p.key ORDER BY MAX(e.at_ms)"

	db, err := openTimerDB(true)
	if err != nil {
		return err
	}
	defer db.Close()
	rows, err := db.Query(query, params...)
	if err != nil {
		return err
	}
	defer rows.Close()

	result := statsResult{
		From:     fromRaw,
		To:       toRaw,
		Accepted: acceptedOnly,
		Problems: []statsProblem{},
		PerDay:   []statsDay{},
	}
	dayMap := map[string]*statsDay{}
	for rows.Next() {
		var problem statsProblem
		var accepted int
		if err = rows.Scan(&problem.Key, &problem.Display, &problem.Name, &problem.Contest,
			&problem.Status, &problem.Practice, &problem.TotalMS, &problem.LastEnd, &accepted); err != nil {
			return err
		}
		problem.Accepted = accepted != 0
		if problem.Accepted {
			result.AcceptedN++
		}
		if acceptedOnly && !problem.Accepted {
			continue
		}
		result.Problems = append(result.Problems, problem)
		result.Count++
		result.TotalMS += problem.TotalMS
		day := time.UnixMilli(problem.LastEnd).Format("2006-01-02")
		bucket := dayMap[day]
		if bucket == nil {
			bucket = &statsDay{Day: day}
			dayMap[day] = bucket
		}
		bucket.Count++
		bucket.TotalMS += problem.TotalMS
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for _, bucket := range dayMap {
		result.PerDay = append(result.PerDay, *bucket)
	}
	sort.Slice(result.PerDay, func(i, j int) bool { return result.PerDay[i].Day < result.PerDay[j].Day })

	if jsonOut {
		return printDBJSON(result)
	}
	printDBStats(&result)
	return nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func dbStatsFilters(specifiers []string) (problemKeys, contestKeys []string) {
	for _, specifier := range specifiers {
		info, err := resolveTimerInfo(specifier)
		if err != nil {
			continue
		}
		keys := keysFromInfo(info)
		if keys.ProblemKey != "" {
			problemKeys = append(problemKeys, keys.ProblemKey)
		} else if keys.ContestKey != "" {
			contestKeys = append(contestKeys, keys.ContestKey)
		}
	}
	return
}

func printDBStats(result *statsResult) {
	rangeText := "all time"
	if result.From != "" || result.To != "" {
		rangeText = "range"
		if result.From != "" {
			rangeText = result.From
		} else {
			rangeText = "..."
		}
		rangeText += " to "
		if result.To != "" {
			rangeText += result.To
		} else {
			rangeText += "today"
		}
	}
	if result.Count == 0 {
		fmt.Printf("No problems completed (%v).\n", rangeText)
		return
	}
	color.Cyan("Completed %v problem(s), accepted %v (%v).", result.Count, result.AcceptedN, rangeText)
	color.Green("Total time: %v", formatElapsedMS(result.TotalMS))
	table := tablewriter.NewWriter(color.Output)
	table.SetHeader([]string{"problem", "name", "contest", "status", "time", "accepted"})
	table.SetAutoWrapText(false)
	table.SetRowLine(false)
	for _, problem := range result.Problems {
		accepted := ""
		if problem.Accepted {
			accepted = "yes"
		}
		table.Append([]string{problem.Display, problem.Name, problem.Contest, problem.Status, formatElapsedMS(problem.TotalMS), accepted})
	}
	table.Render()
	dayTable := tablewriter.NewWriter(color.Output)
	dayTable.SetHeader([]string{"day", "problems", "time"})
	dayTable.SetAutoWrapText(false)
	dayTable.SetRowLine(false)
	for _, day := range result.PerDay {
		dayTable.Append([]string{day.Day, fmt.Sprint(day.Count), formatElapsedMS(day.TotalMS)})
	}
	dayTable.Render()
}

func dbSyncCommand(specifiers []string, refresh bool) error {
	return dbSync(specifiers, refresh)
}
