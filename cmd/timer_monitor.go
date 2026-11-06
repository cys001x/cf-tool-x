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
	"unicode/utf8"

	"github.com/fatih/color"
	ansi "github.com/k0kubun/go-ansi"
	"golang.org/x/term"
)

// timerMonitorStatus draws the ticking line below the command output.
type timerMonitorStatus struct {
	raw    bool
	prompt bool
	length int
	shown  bool
}

func (s *timerMonitorStatus) ending() string {
	if s.raw {
		// Raw mode disables output post-processing, so "\n" alone does not
		// return the cursor to column 0.
		return "\r\n"
	}
	return "\n"
}

// draw prints the status line. Consecutive calls overwrite the same line.
func (s *timerMonitorStatus) draw(st *timeState, input []byte) {
	elapsed := formatElapsedMS(st.elapsed(time.Now()))
	plain := fmt.Sprintf("[%v] %v", elapsed, openProblemNames(st))
	line := fmt.Sprintf("[%v] %v", color.GreenString(elapsed), openProblemNames(st))
	if st.Paused {
		plain += " paused"
		line += " paused"
	}
	plainLine := plain
	if s.prompt {
		plainLine = plain + "  > " + string(input)
		line += "  > " + string(input)
	}
	padding := ""
	if s.length > len(plainLine) {
		padding = strings.Repeat(" ", s.length-len(plainLine))
	}
	if s.shown {
		ansi.CursorUp(1)
	}
	ansi.Printf("%v%v%v", line, padding, s.ending())
	s.shown = true
	s.length = len(plainLine)
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
		return nil, errors.New("The timer is already open. Use `q` to close it, or s/e/p to control it")
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

	color.Cyan("Timer opened. Commands: s/start <specifier> <note>, e/end <specifier> <note>, p/pause, q.")
	color.Cyan("Any other line runs as a cf command below the timer, e.g. `submit 1136a`.")

	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stdout.Fd())) {
		return runTimeMonitorLines()
	}
	return runTimeMonitorInteractive(fd)
}

// runTimeMonitorInteractive owns stdin completely: it polls for input instead
// of using a reader goroutine, so an external command can read stdin while the
// monitor is not reading it.
func runTimeMonitorInteractive(fd int) error {
	status := &timerMonitorStatus{prompt: true}
	rawMode := false
	var termState *term.State

	enterRaw := func() {
		if rawMode {
			return
		}
		if termState != nil {
			// An external command may have changed the terminal mode; restore
			// the state saved before the first raw switch, then go raw again.
			term.Restore(fd, termState)
		}
		if state, err := term.MakeRaw(fd); err == nil {
			termState = state
			rawMode = true
			status.raw = true
		}
	}
	leaveRaw := func() {
		if rawMode {
			term.Restore(fd, termState)
			rawMode = false
			status.raw = false
		}
	}
	enterRaw()
	defer leaveRaw()

	var inputBuf []byte
	lastNotice := int64(0)
	if st, err := loadTimeState(); err == nil {
		lastNotice = st.NoticeSeq
		status.draw(st, inputBuf)
	}

	// runTypedLine executes one line and updates the display.
	runTypedLine := func(line string) (quit bool) {
		leaveRaw()
		status.clear()
		done, external := runMonitorCommandLine(line)
		if done {
			return true
		}
		cur, err := loadTimeState()
		if err != nil {
			return true
		}
		if !cur.Running {
			// The external "submit" already printed the summary after closing
			// the timer, so do not repeat it here.
			leaveRaw()
			return true
		}
		if !external {
			lastNotice = cur.NoticeSeq
		}
		enterRaw()
		status.draw(cur, inputBuf)
		return false
	}

	handleKey := func(b byte) (quit bool) {
		switch {
		case b == 27:
			// Swallow ANSI escape sequences such as arrow keys.
			for stdinReadable(fd, 15*time.Millisecond) {
				var next [1]byte
				n, err := os.Stdin.Read(next[:])
				if err != nil || n == 0 {
					break
				}
				if next[0] >= 0x40 && next[0] <= 0x7e {
					break
				}
			}
		case b == 3 || b == 4:
			leaveRaw()
			fmt.Println()
			color.Yellow("Left the timer running. Use `cf timer q` to close it.")
			return true
		case b == '\r' || b == '\n':
			line := string(inputBuf)
			inputBuf = nil
			if strings.TrimSpace(line) != "" {
				return runTypedLine(line)
			}
		case b == 127 || b == 8:
			if len(inputBuf) > 0 {
				_, size := utf8.DecodeLastRune(inputBuf)
				inputBuf = inputBuf[:len(inputBuf)-size]
			}
		case b >= 32:
			inputBuf = append(inputBuf, b)
		}
		if cur, err := loadTimeState(); err == nil {
			status.draw(cur, inputBuf)
		}
		return false
	}

	inputClosed := false
	nextTick := time.Now().Add(time.Second)
	for {
		if inputClosed {
			time.Sleep(time.Until(nextTick))
		} else {
			wait := time.Until(nextTick)
			if wait < 0 {
				wait = 0
			}
			if stdinReadable(fd, wait) {
				var buf [1]byte
				n, err := os.Stdin.Read(buf[:])
				if n > 0 {
					if handleKey(buf[0]) {
						return nil
					}
				}
				if err != nil {
					inputClosed = true
					status.clear()
					leaveRaw()
					color.Yellow("Input closed. The timer keeps running in the background.")
				}
				continue
			}
		}
		if !time.Now().Before(nextTick) {
			nextTick = time.Now().Add(time.Second)
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
				leaveRaw()
				printTimeSummary(cur)
				return nil
			}
			status.draw(cur, inputBuf)
		}
	}
}

// runTimeMonitorLines handles a redirected stdin. It reads commands one line
// at a time without a reader goroutine so a command can read stdin itself.
func runTimeMonitorLines() error {
	status := &timerMonitorStatus{}
	draw := term.IsTerminal(int(os.Stdout.Fd()))
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
				status.draw(cur, nil)
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
			status.draw(cur, nil)
		}
	}
}
