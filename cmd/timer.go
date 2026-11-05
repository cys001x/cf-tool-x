package cmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/cys001x/cf-tool-x/util"
	"github.com/fatih/color"
	ansi "github.com/k0kubun/go-ansi"
	"github.com/mitchellh/go-homedir"
	"github.com/shirou/gopsutil/process"
	"golang.org/x/term"
)

// TimeRecordFileName is the timer data file written into every problem folder.
const TimeRecordFileName = "count_time.json"

// Timer event types.
const (
	TimeEventStart  = "start"
	TimeEventEnd    = "end"
	TimeEventTest   = "test"
	TimeEventSubmit = "submit"
)

const timeLayout = time.RFC3339

// timeEvent is one recorded time node. Elapsed is relative to the timer
// session start and excludes paused periods.
type timeEvent struct {
	Type string `json:"type"`
	Time string `json:"time"`
	// Elapsed is written as "HH:MM:SS"; ElapsedMS is kept only for records
	// written by older versions.
	Elapsed   string `json:"elapsed,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
	Note      string `json:"note,omitempty"`
}

// problemTimeRecord is stored in "<problem>/count_time.json".
type problemTimeRecord struct {
	Problem string      `json:"problem"`
	Link    string      `json:"link,omitempty"`
	Summary string      `json:"summary,omitempty"`
	Events  []timeEvent `json:"events"`
}

// timeSegment is the problem that is currently being timed.
type timeSegment struct {
	Specifier      string `json:"specifier"`
	Problem        string `json:"problem"`
	Link           string `json:"link"`
	Dir            string `json:"dir"`
	Start          string `json:"start"`
	StartElapsedMS int64  `json:"start_elapsed_ms"`
}

// timeState is the global singleton timer state (~/.cf/timer.json).
type timeState struct {
	Running      bool          `json:"running"`
	Paused       bool          `json:"paused"`
	SessionStart string        `json:"session_start"`
	ElapsedMS    int64         `json:"elapsed_ms"`
	RunningSince string        `json:"running_since,omitempty"`
	MonitorPID   int32         `json:"monitor_pid,omitempty"`
	Open         []timeSegment `json:"open,omitempty"`
	Records      []string      `json:"records,omitempty"`
	Summary      string        `json:"summary,omitempty"`
	Notice       string        `json:"notice,omitempty"`
	NoticeSeq    int64         `json:"notice_seq,omitempty"`
}

// setNotice queues a message for the foreground monitor.
func setNotice(st *timeState, message string) {
	st.Notice = message
	st.NoticeSeq++
}

func timerStatePath() string {
	if path := strings.TrimSpace(os.Getenv("CF_TIMER_STATE")); path != "" {
		return path
	}
	path, _ := homedir.Expand("~/.cf/timer.json")
	return path
}

func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}
	tmp, err := ioutil.TempFile(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err = tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err = os.Rename(name, path); err != nil {
		// Windows cannot rename over an existing file.
		if os.Remove(path) == nil {
			err = os.Rename(name, path)
		}
	}
	if err != nil {
		os.Remove(name)
	}
	return err
}

func loadTimeState() (*timeState, error) {
	b, err := ioutil.ReadFile(timerStatePath())
	if err != nil {
		return nil, err
	}
	st := &timeState{}
	if err = json.Unmarshal(b, st); err != nil {
		return nil, err
	}
	return st, nil
}

func saveTimeState(st *timeState) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(timerStatePath(), append(b, '\n'))
}

// lockTimeState serializes updates from different cf processes.
func lockTimeState() (func(), error) {
	lockPath := timerStatePath() + ".lock"
	os.MkdirAll(filepath.Dir(lockPath), os.ModePerm)
	for i := 0; i < 150; i++ {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			f.Close()
			return func() { os.Remove(lockPath) }, nil
		}
		if info, e := os.Stat(lockPath); e == nil && time.Since(info.ModTime()) > 10*time.Second {
			os.Remove(lockPath)
			continue
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, errors.New("Cannot lock the timer state")
}

func newTimeState(now time.Time) *timeState {
	return &timeState{
		Running:      true,
		SessionStart: now.Format(timeLayout),
		RunningSince: now.Format(timeLayout),
	}
}

// mutateTimeState locks the state, applies fn and saves the result.
func mutateTimeState(createIfMissing bool, fn func(*timeState, time.Time) error) (*timeState, error) {
	release, err := lockTimeState()
	if err != nil {
		return nil, err
	}
	defer release()

	st, err := loadTimeState()
	if err != nil {
		if !createIfMissing {
			return nil, errors.New("No timer. Run `cf timer` or `cf timer start <specifier> <note>` first")
		}
		st = newTimeState(time.Now())
	}
	now := time.Now()
	if err = fn(st, now); err != nil {
		return nil, err
	}
	if err = saveTimeState(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (st *timeState) elapsed(now time.Time) int64 {
	ms := st.ElapsedMS
	if !st.Paused && st.RunningSince != "" {
		if since, err := time.Parse(timeLayout, st.RunningSince); err == nil {
			ms += now.Sub(since).Milliseconds()
		}
	}
	if ms < 0 {
		return 0
	}
	return ms
}

func formatElapsedMS(ms int64) string {
	seconds := ms / 1000
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, seconds%3600/60, seconds%60)
}

func formatEventTime(raw string) string {
	tm, err := time.Parse(timeLayout, raw)
	if err != nil {
		return raw
	}
	return tm.Local().Format("15:04:05")
}

func printTimeEvent(ev timeEvent) {
	note := ""
	if ev.Note != "" {
		note = " (" + ev.Note + ")"
	}
	elapsed := ev.Elapsed
	if elapsed == "" {
		elapsed = formatElapsedMS(ev.ElapsedMS)
	}
	fmt.Printf("  %-7v %v  +%v%v\n", ev.Type, formatEventTime(ev.Time), elapsed, note)
}

func loadProblemTimeRecord(path string) (*problemTimeRecord, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	rec := &problemTimeRecord{}
	if err = json.Unmarshal(b, rec); err != nil {
		return nil, err
	}
	return rec, nil
}

// appendProblemEvent appends one node to "<dir>/count_time.json".
func appendProblemEvent(dir, problem, link string, ev timeEvent) error {
	path := filepath.Join(dir, TimeRecordFileName)
	rec := &problemTimeRecord{}
	if b, err := ioutil.ReadFile(path); err == nil {
		if err = json.Unmarshal(b, rec); err != nil {
			rec = &problemTimeRecord{}
		}
	}
	rec.Problem = problem
	if link != "" {
		rec.Link = link
	}
	rec.Events = append(rec.Events, ev)
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'))
}

// setProblemSummary stores the summary in "<dir>/count_time.json".
func setProblemSummary(dir, problem, link, summary string) error {
	if summary == "" {
		return nil
	}
	path := filepath.Join(dir, TimeRecordFileName)
	rec := &problemTimeRecord{}
	if b, err := ioutil.ReadFile(path); err == nil {
		if err = json.Unmarshal(b, rec); err != nil {
			rec = &problemTimeRecord{}
		}
	}
	rec.Problem = problem
	if link != "" {
		rec.Link = link
	}
	rec.Summary = summary
	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(b, '\n'))
}

func addRecordPath(records []string, path string) []string {
	for _, p := range records {
		if p == path {
			return records
		}
	}
	return append(records, path)
}

// resolveTimerInfo parses one specifier into a problem or contest Info.
func resolveTimerInfo(specifier string) (client.Info, error) {
	specifier = strings.TrimSpace(specifier)
	if specifier == "" {
		return client.Info{}, errors.New("Missing specifier, e.g. `cf timer start 1136a <note>`")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return client.Info{}, err
	}
	info, err := infoFromSpecifiers([]string{specifier}, cwd)
	if err != nil {
		return client.Info{}, err
	}
	if info.ContestID == "" {
		return client.Info{}, fmt.Errorf("Cannot understand the specifier %q", specifier)
	}
	return info, nil
}

func timerLink(info *client.Info) string {
	host := config.Instance.Host
	if info.ProblemID != "" {
		if URL, err := info.ProblemURL(host); err == nil {
			return URL
		}
	}
	if URL, err := info.ProblemSetURL(host); err == nil {
		return URL
	}
	return ""
}

func timerProblemName(specifier string) string {
	return strings.ToLower(strings.Join(strings.Fields(specifier), " "))
}

// timerStart records the start node of a problem in the global timer.
func timerStart(specifier, note string) error {
	info, err := resolveTimerInfo(specifier)
	if err != nil {
		return err
	}
	dir := filepath.Clean(info.Path())
	problem := timerProblemName(specifier)
	link := timerLink(&info)

	_, err = mutateTimeState(true, func(st *timeState, now time.Time) error {
		if !st.Running {
			*st = *newTimeState(now)
		}
		for _, seg := range st.Open {
			if seg.Dir == dir {
				return fmt.Errorf("Problem %v is already running. Run `cf timer end %v <note>` first",
					seg.Problem, seg.Specifier)
			}
		}
		elapsed := st.elapsed(now)
		ev := timeEvent{Type: TimeEventStart, Time: now.Format(timeLayout), Elapsed: formatElapsedMS(elapsed), Note: note}
		if err := appendProblemEvent(dir, problem, link, ev); err != nil {
			return err
		}
		st.Open = append(st.Open, timeSegment{
			Specifier:      specifier,
			Problem:        problem,
			Link:           link,
			Dir:            dir,
			Start:          now.Format(timeLayout),
			StartElapsedMS: elapsed,
		})
		st.Records = addRecordPath(st.Records, filepath.Join(dir, TimeRecordFileName))
		setNotice(st, fmt.Sprintf("start %v", problem))
		return nil
	})
	if err != nil {
		return err
	}
	color.Green("Started %v.", problem)
	return nil
}

// timerEnd records the end node of the problem opened by a matching start.
func timerEnd(specifier, note string) error {
	if strings.TrimSpace(specifier) == "" {
		return errors.New("Missing specifier, e.g. `cf timer end 1136a <note>`")
	}
	info, err := resolveTimerInfo(specifier)
	if err != nil {
		return err
	}
	dir := filepath.Clean(info.Path())

	_, err = mutateTimeState(false, func(st *timeState, now time.Time) error {
		if !st.Running {
			return errors.New("The timer is not running")
		}
		index := -1
		for i, seg := range st.Open {
			if seg.Dir == dir {
				index = i
				break
			}
		}
		if index < 0 {
			if len(st.Open) == 0 {
				return errors.New("No running problem. Run `cf timer start <specifier> <note>` first")
			}
			names := make([]string, len(st.Open))
			for i, seg := range st.Open {
				names[i] = seg.Problem
			}
			return fmt.Errorf("`end %v` has no matching start. Running problems: %v", specifier, strings.Join(names, ", "))
		}
		seg := st.Open[index]
		total := st.elapsed(now)
		problemElapsed := total - seg.StartElapsedMS
		if problemElapsed < 0 {
			problemElapsed = 0
		}
		ev := timeEvent{Type: TimeEventEnd, Time: now.Format(timeLayout), Elapsed: formatElapsedMS(total), Note: note}
		if err := appendProblemEvent(seg.Dir, seg.Problem, seg.Link, ev); err != nil {
			return err
		}
		if err := setProblemSummary(seg.Dir, seg.Problem, seg.Link, note); err != nil {
			return err
		}
		color.Green("Ended %v after %v (timer total %v).", seg.Problem, formatElapsedMS(problemElapsed), formatElapsedMS(total))
		setNotice(st, fmt.Sprintf("end %v after %v", seg.Problem, formatElapsedMS(problemElapsed)))
		st.Open = append(st.Open[:index], st.Open[index+1:]...)
		return nil
	})
	return err
}

// timerPause pauses or resumes the timer.
func timerPause() error {
	_, err := mutateTimeState(false, func(st *timeState, now time.Time) error {
		if !st.Running {
			return errors.New("The timer is not running")
		}
		if st.Paused {
			st.Paused = false
			st.RunningSince = now.Format(timeLayout)
			setNotice(st, "timer resumed")
			color.Green("Timer resumed.")
			return nil
		}
		st.ElapsedMS = st.elapsed(now)
		st.Paused = true
		st.RunningSince = ""
		setNotice(st, "timer paused")
		color.Green("Timer paused.")
		return nil
	})
	return err
}

// closeTimeSession ends every open segment and closes the timer.
func closeTimeSession(note string) (*timeState, error) {
	return mutateTimeState(false, func(st *timeState, now time.Time) error {
		if !st.Running {
			return errors.New("The timer is not running")
		}
		endNote := note
		if endNote == "" {
			endNote = "auto end on quit"
		}
		elapsed := st.elapsed(now)
		for _, seg := range st.Open {
			ev := timeEvent{Type: TimeEventEnd, Time: now.Format(timeLayout), Elapsed: formatElapsedMS(elapsed), Note: endNote}
			if err := appendProblemEvent(seg.Dir, seg.Problem, seg.Link, ev); err != nil {
				return err
			}
			if err := setProblemSummary(seg.Dir, seg.Problem, seg.Link, endNote); err != nil {
				return err
			}
		}
		st.Open = nil
		st.Summary = note
		st.ElapsedMS = elapsed
		st.Running = false
		st.Paused = false
		st.RunningSince = ""
		st.MonitorPID = 0
		return nil
	})
}

// printTimeSummary prints every recorded problem and its time nodes.
// eventInSession reports whether an event belongs to the current timer session.
// The record files keep every session, so old nodes must be skipped.
func eventInSession(ev timeEvent, sessionStart time.Time) bool {
	if sessionStart.IsZero() {
		return true
	}
	tm, err := time.Parse(timeLayout, ev.Time)
	if err != nil {
		return true
	}
	return !tm.Before(sessionStart)
}

func printTimeSummary(st *timeState) {
	total := st.ElapsedMS
	if st.Running {
		total = st.elapsed(time.Now())
	}
	sessionStart, _ := time.Parse(timeLayout, st.SessionStart)

	color.Cyan("Timer summary (total %v)", formatElapsedMS(total))
	if st.Summary != "" {
		fmt.Printf("  summary: %v\n", st.Summary)
	}
	printed := 0
	for _, path := range st.Records {
		rec, err := loadProblemTimeRecord(path)
		if err != nil {
			fmt.Printf("  %v (cannot read: %v)\n", path, err)
			continue
		}
		events := []timeEvent{}
		hasEnd := false
		for _, ev := range rec.Events {
			if !eventInSession(ev, sessionStart) {
				continue
			}
			events = append(events, ev)
			if ev.Type == TimeEventEnd {
				hasEnd = true
			}
		}
		if len(events) == 0 {
			continue
		}
		if printed > 0 {
			fmt.Println()
		}
		printed++
		fmt.Printf("%v", rec.Problem)
		if rec.Link != "" {
			fmt.Printf("  %v", rec.Link)
		}
		fmt.Println()
		if hasEnd && rec.Summary != "" {
			fmt.Printf("  summary: %v\n", rec.Summary)
		}
		for _, ev := range events {
			printTimeEvent(ev)
		}
	}
	if printed == 0 {
		fmt.Println("  No problems were timed.")
	}
}

func openProblemNames(st *timeState) string {
	if len(st.Open) == 0 {
		return "idle"
	}
	names := make([]string, len(st.Open))
	for i, seg := range st.Open {
		names[i] = seg.Problem
	}
	return strings.Join(names, ", ")
}

func printTimeStatus(st *timeState) {
	now := time.Now()
	state := "running"
	if st.Paused {
		state = "paused"
	}
	if !st.Running {
		state = "closed"
	}
	color.Cyan("Timer %v  %v  %v", formatElapsedMS(st.elapsed(now)), openProblemNames(st), state)
}

// currentProblemDirs lists the folders that can own the current problem timer.
func currentProblemDirs() (dirs []string) {
	info := Args.Info
	info.Dir = ""
	if info.ProblemID != "" {
		dirs = append(dirs, filepath.Clean(info.Path()))
	}
	if Args.Dir != "" {
		dirs = append(dirs, filepath.Clean(Args.Dir))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, filepath.Clean(wd))
	}
	return
}

func dirInList(dir string, list []string) bool {
	for _, item := range list {
		if item == dir {
			return true
		}
	}
	return false
}

// appendAutoTimeEvent records a test or submit node for the open problem that
// matches the current folder.
func appendAutoTimeEvent(eventType, note string) bool {
	release, err := lockTimeState()
	if err != nil {
		return false
	}
	defer release()

	st, err := loadTimeState()
	if err != nil || !st.Running {
		return false
	}
	dirs := currentProblemDirs()
	now := time.Now()
	for _, seg := range st.Open {
		if !dirInList(seg.Dir, dirs) {
			continue
		}
		elapsed := st.elapsed(now)
		ev := timeEvent{Type: eventType, Time: now.Format(timeLayout), Elapsed: formatElapsedMS(elapsed), Note: note}
		if err := appendProblemEvent(seg.Dir, seg.Problem, seg.Link, ev); err != nil {
			return false
		}
		setNotice(st, fmt.Sprintf("%v %v +%v", eventType, seg.Problem, formatElapsedMS(elapsed)))
		if err := saveTimeState(st); err != nil {
			return false
		}
		return true
	}
	return false
}

// recordTestPassed is called by "cf test" when every sample passed.
func recordTestPassed() {
	if appendAutoTimeEvent(TimeEventTest, "all samples passed") {
		color.Green("Timer: all samples passed recorded.")
	}
}

// recordSubmissionTimer records the submission node and asks whether to close
// the timer when the verdict is accepted.
func recordSubmissionTimer(cln *client.Client) {
	if cln == nil {
		return
	}
	note := "verdict " + cln.LastVerdict
	if cln.LastVerdict == "OK" && cln.LastSubmission != nil && cln.LastSubmission.SubmissionID != "" {
		note += " submission " + cln.LastSubmission.SubmissionID
	}
	if !appendAutoTimeEvent(TimeEventSubmit, note) {
		return
	}
	color.Green("Timer: submission recorded.")
	if cln.LastVerdict != "OK" {
		return
	}
	if !util.YesOrNo("Accepted. Close the timer? (y/n) ") {
		return
	}
	color.Cyan("Summary of the session: ")
	note = util.ScanlineTrim()
	st, err := closeTimeSession(note)
	if err != nil {
		color.Red(err.Error())
		return
	}
	color.Green("Timer closed.")
	printTimeSummary(st)
}

// runTimeMonitor renders the ticking timer and handles typed commands.
func runTimeMonitor() error {
	defer func() {
		mutateTimeState(false, func(st *timeState, now time.Time) error {
			if st.MonitorPID == int32(os.Getpid()) {
				st.MonitorPID = 0
			}
			return nil
		})
	}()

	color.Cyan("Timer opened. Commands: s/start <specifier> <note>, e/end <specifier> <note>, p/pause, q.")

	fd := int(os.Stdin.Fd())
	inputTTY := term.IsTerminal(fd)
	// Raw input needs both sides: with a non-TTY stdout the status line is
	// rendered like "cf submit", without terminal output post-processing.
	interactive := inputTTY && term.IsTerminal(int(os.Stdout.Fd()))
	rawMode := false
	var termState *term.State
	enterRaw := func() {
		if !interactive || rawMode {
			return
		}
		if state, err := term.MakeRaw(fd); err == nil {
			termState = state
			rawMode = true
		}
	}
	leaveRaw := func() {
		if rawMode {
			term.Restore(fd, termState)
			rawMode = false
		}
	}
	enterRaw()
	defer leaveRaw()

	keyInput := make(chan byte)
	lineInput := make(chan string)
	if interactive {
		go func() {
			buf := make([]byte, 1)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					keyInput <- buf[0]
				}
				if err != nil {
					close(keyInput)
					return
				}
			}
		}()
	} else {
		go func() {
			reader := bufio.NewReader(os.Stdin)
			for {
				line, err := reader.ReadString('\n')
				if err != nil && line == "" {
					close(lineInput)
					return
				}
				lineInput <- strings.TrimSpace(line)
				if err != nil {
					close(lineInput)
					return
				}
			}
		}()
	}

	var inputBuf []byte
	statusLen := 0
	statusShown := false

	// The ticker uses the same refresh scheme as "cf submit": print the line
	// with a newline and move the cursor back up with go-ansi on the next draw.
	drawStatus := func(st *timeState) {
		elapsed := formatElapsedMS(st.elapsed(time.Now()))
		plain := fmt.Sprintf("[%v] %v", elapsed, openProblemNames(st))
		line := fmt.Sprintf("[%v] %v", color.GreenString(elapsed), openProblemNames(st))
		if st.Paused {
			plain += " paused"
			line += " paused"
		}
		plainLine := plain
		if interactive {
			plainLine = plain + "  > " + string(inputBuf)
			line += "  > " + string(inputBuf)
		}
		padding := ""
		if statusLen > len(plainLine) {
			padding = strings.Repeat(" ", statusLen-len(plainLine))
		}
		if statusShown {
			ansi.CursorUp(1)
		}
		ending := "\n"
		if rawMode {
			// Raw mode disables output post-processing, so "\n" alone does
			// not return the cursor to column 0.
			ending = "\r\n"
		}
		ansi.Printf("%v%v%v", line, padding, ending)
		statusShown = true
		statusLen = len(plainLine)
	}
	printMonitorLine := func(format string, a ...interface{}) {
		msg := color.CyanString(format, a...)
		if rawMode {
			ansi.Printf("%v\r\n", msg)
			return
		}
		fmt.Println(msg)
	}
	clearStatus := func() {
		if !statusShown {
			return
		}
		ansi.CursorUp(1)
		ending := "\n"
		if rawMode {
			ending = "\r\n"
		}
		ansi.Printf("%v%v", strings.Repeat(" ", statusLen), ending)
		ansi.CursorUp(1)
		statusShown = false
	}

	// runLine executes one command line. It returns true when the timer is done.
	runLine := func(line string) bool {
		if strings.TrimSpace(line) == "" {
			return false
		}
		leaveRaw()
		clearStatus()
		err := runTimeCommand(line)
		if err != nil {
			color.Red(err.Error())
		}
		cmd, _, _ := parseTimerArgs(strings.Fields(line))
		done := cmd == "q"
		if !done {
			enterRaw()
		}
		return done
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	st, err := loadTimeState()
	if err != nil {
		return err
	}
	lastNotice := st.NoticeSeq
	drawStatus(st)

	for {
		select {
		case b, ok := <-keyInput:
			if !ok {
				keyInput = nil
				clearStatus()
				leaveRaw()
				color.Yellow("Input closed. The timer keeps running in the background.")
				continue
			}
			switch {
			case b == 27:
				// Swallow ANSI escape sequences such as arrow keys.
				for {
					next, more := <-keyInput
					if !more {
						break
					}
					if next >= 0x40 && next <= 0x7e {
						break
					}
				}
			case b == 3 || b == 4:
				leaveRaw()
				fmt.Println()
				color.Yellow("Left the timer running. Use `cf timer q` to close it.")
				return nil
			case b == '\r' || b == '\n':
				line := string(inputBuf)
				inputBuf = nil
				if line == "" {
					if cur, e := loadTimeState(); e == nil {
						drawStatus(cur)
					}
					continue
				}
				if runLine(line) {
					return nil
				}
				cur, e := loadTimeState()
				if e != nil {
					return e
				}
				lastNotice = cur.NoticeSeq
				if !cur.Running {
					leaveRaw()
					printTimeSummary(cur)
					return nil
				}
				drawStatus(cur)
			case b == 127 || b == 8:
				if len(inputBuf) > 0 {
					_, size := utf8.DecodeLastRune(inputBuf)
					inputBuf = inputBuf[:len(inputBuf)-size]
				}
				if cur, e := loadTimeState(); e == nil {
					drawStatus(cur)
				}
			case b >= 32:
				inputBuf = append(inputBuf, b)
				if cur, e := loadTimeState(); e == nil {
					drawStatus(cur)
				}
			}
		case line, ok := <-lineInput:
			clearStatus()
			if !ok {
				lineInput = nil
				color.Yellow("Input closed. The timer keeps running in the background.")
				continue
			}
			if runLine(line) {
				return nil
			}
			cur, loadErr := loadTimeState()
			if loadErr != nil {
				return loadErr
			}
			lastNotice = cur.NoticeSeq
			if !cur.Running {
				leaveRaw()
				printTimeSummary(cur)
				return nil
			}
			drawStatus(cur)
		case <-ticker.C:
			cur, e := loadTimeState()
			if e != nil {
				continue
			}
			if cur.NoticeSeq != lastNotice && cur.Notice != "" {
				clearStatus()
				printMonitorLine("Timer: %v", cur.Notice)
				lastNotice = cur.NoticeSeq
			}
			if !cur.Running {
				clearStatus()
				leaveRaw()
				printTimeSummary(cur)
				return nil
			}
			drawStatus(cur)
		}
	}
}

// runTimeCommand executes one timer command line (used by the monitor).
func runTimeCommand(line string) error {
	cmd, specifier, note := parseTimerArgs(strings.Fields(line))
	return runTimerCommand(cmd, specifier, note)
}

// normalizeTimerCommand accepts the first letter of start, end, pause and q.
func normalizeTimerCommand(command string) string {
	switch strings.ToLower(command) {
	case "s":
		return "start"
	case "e":
		return "end"
	case "p":
		return "pause"
	case "q", "exit":
		return "q"
	}
	return strings.ToLower(command)
}

// parseTimerArgs splits "<command> <specifier> <note words...>".
func parseTimerArgs(args []string) (command, specifier, note string) {
	rest := args
	if len(rest) > 0 {
		command = normalizeTimerCommand(rest[0])
	}
	if command == "start" || command == "end" {
		if len(rest) > 1 {
			specifier = rest[1]
		}
		if len(rest) > 2 {
			note = strings.Trim(strings.Join(rest[2:], " "), `"'`)
		}
	}
	if command == "q" {
		if len(rest) > 1 {
			note = strings.Trim(strings.Join(rest[1:], " "), `"'`)
		}
	}
	return
}

func runTimerCommand(command, specifier, note string) error {
	switch command {
	case "", "open":
		return timerOpen()
	case "start":
		return timerStart(specifier, note)
	case "end":
		return timerEnd(specifier, note)
	case "pause":
		return timerPause()
	case "q":
		st, err := closeTimeSession(note)
		if err != nil {
			return err
		}
		color.Green("Timer closed.")
		printTimeSummary(st)
		return nil
	case "status":
		st, err := loadTimeState()
		if err != nil {
			return errors.New("No timer")
		}
		printTimeStatus(st)
		return nil
	}
	return fmt.Errorf("Unknown timer command %q. Use s/start, e/end, p/pause, q or status", command)
}

func monitorAlive(pid int32) bool {
	alive, err := process.PidExists(pid)
	return err == nil && alive
}

// timerOpen opens or attaches to the singleton timer.
func timerOpen() error {
	if _, err := loadTimeState(); err != nil {
		if _, err = mutateTimeState(true, func(st *timeState, now time.Time) error { return nil }); err != nil {
			return err
		}
		color.Green("Timer created.")
	}

	st, err := loadTimeState()
	if err != nil {
		return err
	}
	if !st.Running {
		if _, err = mutateTimeState(false, func(st *timeState, now time.Time) error {
			*st = *newTimeState(now)
			return nil
		}); err != nil {
			return err
		}
		if st, err = loadTimeState(); err != nil {
			return err
		}
		color.Green("New timer created.")
	}
	if st.MonitorPID > 0 && st.MonitorPID != int32(os.Getpid()) && monitorAlive(st.MonitorPID) {
		color.Yellow("The timer is already open in process %v.", st.MonitorPID)
		printTimeStatus(st)
		color.Cyan("Control it from any terminal with `cf timer start/end/pause/q`.")
		return nil
	}
	if _, err = mutateTimeState(false, func(st *timeState, now time.Time) error {
		st.MonitorPID = int32(os.Getpid())
		return nil
	}); err != nil {
		return err
	}
	return runTimeMonitor()
}

// Timer command
func Timer() (err error) {
	command, specifier, note := parseTimerArgs(Args.CountArgs)
	return runTimerCommand(command, specifier, note)
}
