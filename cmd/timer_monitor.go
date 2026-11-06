package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"time"
	"unicode"

	"github.com/fatih/color"
	ansi "github.com/k0kubun/go-ansi"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

// timerMonitorStatus draws the ticking line below the command output.
type timerMonitorStatus struct {
	raw    bool
	length int
	width  int
	shown  bool
}

func terminalWidth(fd int) int {
	width, _, err := term.GetSize(fd)
	if err != nil || width < 1 {
		return 0
	}
	return width
}

func (s *timerMonitorStatus) ending() string {
	if s.raw {
		// Raw mode disables output post-processing, so "\n" alone does not
		// return the cursor to column 0.
		return "\r\n"
	}
	return "\n"
}

// draw prints the status line. Consecutive calls overwrite the same line. The
// line is clipped to the terminal width so it never wraps.
func (s *timerMonitorStatus) draw(st *timeState) {
	elapsed := formatElapsedMS(st.elapsed(time.Now()))
	plainLine := fmt.Sprintf("[%v] %v", elapsed, openProblemNames(st))
	if st.Paused {
		plainLine += " paused"
	}
	line := fmt.Sprintf("[%v] %v", color.GreenString(elapsed), openProblemNames(st))
	if st.Paused {
		line += " paused"
	}
	if s.width > 0 {
		// Leave the last column unused: writing exactly one full row can make
		// some terminals scroll before the trailing newline.
		max := s.width - 1
		if max < 1 {
			max = 1
		}
		if runewidth.StringWidth(plainLine) > max {
			plainLine = runewidth.Truncate(plainLine, max, "…")
			line = plainLine
		}
	}
	width := runewidth.StringWidth(plainLine)
	padding := ""
	if s.length > width {
		padding = strings.Repeat(" ", s.length-width)
	}
	if s.shown {
		ansi.CursorUp(1)
	}
	ansi.Printf("%v%v%v", line, padding, s.ending())
	s.shown = true
	s.length = width
}

// clear blanks the status line and parks the cursor on it so command output
// starts there.
func (s *timerMonitorStatus) clear() {
	if !s.shown {
		return
	}
	ansi.CursorUp(1)
	ansi.Printf("%v%v", strings.Repeat(" ", s.length), s.ending())
	ansi.CursorUp(1)
	s.shown = false
}

func (s *timerMonitorStatus) notice(format string, a ...interface{}) {
	msg := color.CyanString(format, a...)
	if s.raw {
		ansi.Printf("%v\r\n", msg)
		return
	}
	fmt.Println(msg)
}

// splitCommandLine splits a command line on spaces and honors single and
// double quotes.
func splitCommandLine(line string) (args []string) {
	var cur strings.Builder
	var quote rune
	escaped := false
	inArg := false
	for _, r := range line {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
			inArg = true
		case r == '\\' && quote != '\'':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inArg = true
		case unicode.IsSpace(r):
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if escaped {
		cur.WriteRune('\\')
		inArg = true
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args
}

// isTimerMonitorCommand reports whether the first word is a command handled by
// the timer itself. Everything else runs as a normal "cf" command.
func isTimerMonitorCommand(command string) bool {
	switch normalizeTimerCommand(command) {
	case "start", "end", "pause", "q", "status":
		return true
	}
	return false
}

// monitorCfArgs normalizes the command line typed in the monitor. A leading
// "cf" is optional, and the timer commands are rejected because the timer is
// already open.
func monitorCfArgs(args []string) ([]string, error) {
	if len(args) > 0 && strings.EqualFold(args[0], "cf") {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil, errors.New("Missing command")
	}
	switch strings.ToLower(args[0]) {
	case "timer", "counttime":
		return nil, errors.New("`timer` is not a command. Use s/start and e/end inside the shell, or `cf start`/`cf end` from any terminal")
	}
	return args, nil
}

// cfCommandRunner runs one cf command in a child process. It is a variable so
// tests can replace it.
var cfCommandRunner = func(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(exe, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func runCfCommand(args []string) error {
	argv, err := monitorCfArgs(args)
	if err != nil {
		return err
	}
	return cfCommandRunner(argv)
}

// runMonitorCommandLine executes one line typed in the monitor and returns
// whether the timer was closed and whether an external cf command ran.
func runMonitorCommandLine(line string) (quit, external bool) {
	args := splitCommandLine(line)
	if len(args) == 0 {
		return false, false
	}
	fmt.Printf("%v %v\n", color.CyanString(">"), line)
	if isTimerMonitorCommand(args[0]) {
		command, practice, specifier, note := parseTimerArgs(args)
		if err := runTimerCommand(command, practice, specifier, note); err != nil {
			color.Red(err.Error())
		}
		return command == "q", false
	}
	// The terminal is in cooked mode while the child runs, so Ctrl-C raises
	// SIGINT for the whole process group. Ignore it here; the child exits on
	// it and the timer must keep running.
	stop := ignoreInterrupt()
	err := runCfCommand(args)
	stop()
	if err != nil && !strings.Contains(err.Error(), "interrupt") {
		color.Red(err.Error())
	}
	return false, true
}

// ignoreInterrupt drains SIGINT until the returned stop function is called,
// then restores the default behavior.
func ignoreInterrupt() (stop func()) {
	interrupts := make(chan os.Signal, 8)
	done := make(chan struct{})
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for {
			select {
			case <-interrupts:
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(interrupts)
		close(done)
	}
}

// runTimeMonitor renders the ticking timer and runs typed commands. Besides
// the timer commands, every other line is executed as a normal "cf" command
// in a child process, so the timer stays open while the command runs.
func runTimeMonitor() error {
	defer func() {
		mutateTimeState(false, func(st *timeState, now time.Time) error {
			if st.MonitorPID == int32(os.Getpid()) {
				st.MonitorPID = 0
			}
			return nil
		})
	}()

	// While an external command runs the terminal is in cooked mode and
	// Ctrl-C raises SIGINT for the process group. The runner ignores it in the
	// monitor and lets the child exit; see ignoreInterrupt.

	color.Cyan("cf shell opened. Timer commands: s/start <specifier> <note>, e/end <specifier> <note>, p/pause, q.")
	color.Cyan("Any other line runs as a cf command below the timer, e.g. `submit 1136a`. `cf --help` shows all commands.")

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return runTimeMonitorLines()
	}
	return runReplTUI()
}

// runTimeMonitorLines handles a redirected stdin. It reads commands one line
// at a time without a reader goroutine so a command can read stdin itself.
func runTimeMonitorLines() error {
	status := &timerMonitorStatus{}
	draw := term.IsTerminal(int(os.Stdout.Fd()))
	if draw {
		status.width = terminalWidth(int(os.Stdout.Fd()))
	}
	reader := bufio.NewReader(os.Stdin)
	lastNotice := int64(0)
	if st, err := loadTimeState(); err == nil {
		lastNotice = st.NoticeSeq
	}
	for {
		line, err := reader.ReadString('\n')
		if line = strings.TrimSpace(line); line != "" {
			quit, external := runMonitorCommandLine(line)
			if quit {
				return nil
			}
			cur, loadErr := loadTimeState()
			if loadErr != nil {
				return loadErr
			}
			if !cur.Running {
				status.clear()
				// An external "submit" already printed the summary.
				if !external {
					printTimeSummary(cur)
				}
				return nil
			}
			lastNotice = cur.NoticeSeq
			if draw {
				status.draw(cur)
			}
		}
		if err != nil {
			color.Yellow("Input closed. The timer keeps running in the background.")
			break
		}
	}
	// Keep showing the timer until it is closed from another terminal.
	for {
		time.Sleep(time.Second)
		cur, err := loadTimeState()
		if err != nil {
			continue
		}
		if cur.NoticeSeq != lastNotice && cur.Notice != "" {
			status.clear()
			status.notice("Timer: %v", cur.Notice)
			lastNotice = cur.NoticeSeq
		}
		if !cur.Running {
			status.clear()
			printTimeSummary(cur)
			return nil
		}
		if draw {
			status.draw(cur)
		}
	}
}
