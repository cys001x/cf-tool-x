package cmd

import (
	"os"
	"reflect"
	"testing"
)

func TestSplitCommandLine(t *testing.T) {
	cases := []struct {
		line string
		want []string
	}{
		{"submit 1136a", []string{"submit", "1136a"}},
		{`submit -f "a b.cpp" 1136a`, []string{"submit", "-f", "a b.cpp", "1136a"}},
		{`submit -f 'a b.cpp'`, []string{"submit", "-f", "a b.cpp"}},
		{`db sql "SELECT * FROM problem"`, []string{"db", "sql", "SELECT * FROM problem"}},
		{`note "with \"quotes\""`, []string{"note", `with "quotes"`}},
		{`a\ b c`, []string{"a b", "c"}},
		{"", nil},
		{"   ", nil},
	}
	for _, c := range cases {
		if got := splitCommandLine(c.line); !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCommandLine(%q) = %#v, want %#v", c.line, got, c.want)
		}
	}
}

func TestIsTimerMonitorCommand(t *testing.T) {
	timerCommands := []string{"s", "start", "e", "end", "p", "pause", "q", "exit", "status"}
	for _, command := range timerCommands {
		if !isTimerMonitorCommand(command) {
			t.Errorf("isTimerMonitorCommand(%q) = false, want true", command)
		}
	}
	cfCommands := []string{"submit", "test", "open", "db", "lpractice", "list", "timer"}
	for _, command := range cfCommands {
		if isTimerMonitorCommand(command) {
			t.Errorf("isTimerMonitorCommand(%q) = true, want false", command)
		}
	}
}

func TestMonitorCfArgs(t *testing.T) {
	got, err := monitorCfArgs([]string{"cf", "submit", "1136a"})
	if err != nil || !reflect.DeepEqual(got, []string{"submit", "1136a"}) {
		t.Fatalf("monitorCfArgs = %#v, %v", got, err)
	}
	if _, err = monitorCfArgs([]string{"timer"}); err == nil {
		t.Fatal("timer must be rejected inside the timer")
	}
	if _, err = monitorCfArgs([]string{"countTime"}); err == nil {
		t.Fatal("countTime must be rejected inside the timer")
	}
	if _, err = monitorCfArgs(nil); err == nil {
		t.Fatal("empty command must be rejected")
	}
	if _, err = monitorCfArgs([]string{"cf"}); err == nil {
		t.Fatal("bare cf must be rejected")
	}
}

func TestRunMonitorCommandLineExternal(t *testing.T) {
	setupTimerTest(t)
	old := cfCommandRunner
	t.Cleanup(func() { cfCommandRunner = old })
	var calls [][]string
	cfCommandRunner = func(args []string) error {
		calls = append(calls, args)
		return nil
	}

	quit, external := runMonitorCommandLine(`submit -f "a b.cpp" 1136a`)
	if quit || !external {
		t.Fatalf("runMonitorCommandLine = (quit=%v, external=%v)", quit, external)
	}
	if len(calls) != 1 || !reflect.DeepEqual(calls[0], []string{"submit", "-f", "a b.cpp", "1136a"}) {
		t.Fatalf("runner calls = %#v", calls)
	}

	// "open" is a cf command inside the timer, not the internal timerOpen.
	if _, external = runMonitorCommandLine("open 1136a"); !external || len(calls) != 2 {
		t.Fatalf("open should run as a cf command: %#v", calls)
	}

	// The timer command is rejected and must not reach the runner.
	runMonitorCommandLine("timer")
	if len(calls) != 2 {
		t.Fatalf("timer must not reach the runner: %#v", calls)
	}
}

func TestRunMonitorCommandLineTimerCommands(t *testing.T) {
	setupTimerTest(t)

	quit, external := runMonitorCommandLine("s 1136a note")
	if quit || external {
		t.Fatalf("start = (quit=%v, external=%v)", quit, external)
	}
	st, err := loadTimeState()
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running || len(st.Open) != 1 || st.Open[0].Problem != "1136a" {
		t.Fatalf("unexpected state after start: %+v", st)
	}

	if quit, external = runMonitorCommandLine("e 1136a done"); quit || external {
		t.Fatalf("end = (quit=%v, external=%v)", quit, external)
	}
	st, _ = loadTimeState()
	if len(st.Open) != 0 {
		t.Fatalf("open segments after end: %+v", st.Open)
	}

	if quit, external = runMonitorCommandLine("q"); !quit || external {
		t.Fatalf("q = (quit=%v, external=%v)", quit, external)
	}
	st, _ = loadTimeState()
	if st.Running {
		t.Fatal("timer should be closed")
	}
}

func TestTimerCommandOnlyStartAndEnd(t *testing.T) {
	setupTimerTest(t)
	for _, command := range []string{"pause", "p", "q", "exit", "status", ""} {
		Args.CommandArgs = []string{command}
		if err := Timer(); err == nil {
			t.Errorf("`cf timer %q` must be rejected", command)
		}
	}
}

func TestBareTimerCommandRequiresSubcommand(t *testing.T) {
	setupTimerTest(t)
	if err := runTimerCommand("", "", "", ""); err == nil {
		t.Fatal("bare `cf timer` must not open the shell")
	}
	if err := runTimerCommand("open", "", "", ""); err == nil {
		t.Fatal("`cf timer open` must be rejected")
	}
	if _, err := loadTimeState(); err == nil {
		t.Fatal("bare `cf timer` must not create timer state")
	}
}

func TestRunTimeMonitorLines(t *testing.T) {
	setupTimerTest(t)
	if err := timerStart("", "1136a", "note"); err != nil {
		t.Fatal(err)
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = reader
	t.Cleanup(func() { os.Stdin = oldStdin })

	if _, err = writer.WriteString("status\nq\n"); err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	if err = runTimeMonitor(); err != nil {
		t.Fatal(err)
	}
	st, err := loadTimeState()
	if err != nil {
		t.Fatal(err)
	}
	if st.Running {
		t.Fatal("q should close the timer")
	}
}
