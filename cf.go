package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cys001x/cf-tool-x/client"
	"github.com/cys001x/cf-tool-x/cmd"
	"github.com/cys001x/cf-tool-x/config"
	"github.com/fatih/color"
	ansi "github.com/k0kubun/go-ansi"
	"github.com/mitchellh/go-homedir"

	docopt "github.com/docopt/docopt-go"
)

const version = "v1.3.4"
const configPath = "~/.cf/config"
const sessionPath = "~/.cf/session"

func main() {
	if len(os.Args) > 1 {
		switch strings.ToLower(os.Args[1]) {
		case "lpractice":
			os.Args[1] = "lpractice"
		case "lrepractice", "lrepractive":
			os.Args[1] = "lrepractice"
		case "lnrepractice", "lnrepractive":
			os.Args[1] = "lnrepractice"
		case "opractice":
			os.Args[1] = "opractice"
		}
		if message := cmd.ValidateCommandLine(os.Args[1:]); message != "" {
			fmt.Fprintln(os.Stderr, message)
			os.Exit(1)
		}
	}
	usage := `Codeforces Tool $%version%$ (cf). https://github.com/cys001x/cf-tool-x

You should run "cf config" to configure your handle, password and code
templates at first.

If you want to compete, the best command is "cf race"

Usage:
  cf
  cf config
  cf submit [-f <file>] [<specifier>...]
  cf list [<specifier>...]
  cf ncontests
  cf parse [<specifier>...]
  cf gen [<alias>]
  cf test [-f <file>] [-d <dir>] [<specifier>...]
  cf watch [all] [<specifier>...]
  cf open [<specifier>...]
  cf stand [<specifier>...]
  cf sid [<specifier>...]
  cf detail [<specifier>...]
  cf race [<specifier>...]
  cf pull [ac] [<specifier>...]
  cf sync [<specifier>...]
  cf start [<args>...]
  cf end [<args>...]
  cf db [options] [<args>...]
  cf lpractice [options] <count> [<name>]
  cf lrepractice [options] <count> [<name>]
  cf lnrepractice [options] <count> [<name>]
  cf opractice [options] <count> [<name>]
  cf clone [ac] [<handle>]
  cf statement [<specifier>...] [-d <dir>] [-f <file>]
  cf material [<specifier>...] [-d <dir>] [-f <file>]
  cf completion [<shell>]
  cf upgrade

Options:
  -h --help            Show this screen.
  --version            Show version.
  -f <file>, --file <file>, <file>
                       Path to file. E.g. "a.cpp", "./temp/a.cpp"
                       Download file name.
  -d <dir>, --dir <dir>
                       Path to directory containing sample files for cf test.
                       Download file Path.
  <specifier>          Any useful text. E.g.
                       "https://codeforces.com/contest/100",
                       "https://codeforces.com/contest/180/problem/A",
                       "https://codeforces.com/group/Cw4JRyRGXR/contest/269760"
                       "1111A", "1111", "a", "Cw4JRyRGXR"
                       "p_drill_1136a" means problem 1136a in
                       "{cf}/practice/drill" (a practice set created by
                       "cf lpractice"/"cf opractice").
                       You can combine multiple specifiers to specify what you
                       want.
  <alias>              Template's alias. E.g. "cpp"
  <shell>              Shell to generate completion for: bash, zsh or fish.
  <args>               Arguments of "cf start"/"cf end": an optional
                       "p_<set>" practice set, a specifier (or a
                       "p_<set>_<problem>" practice specifier), and a note.
                       For "cf db": a subcommand (sql/time/stats/sync/path)
                       and its arguments.
  ac                   The status of the submission is Accepted.
  --from <date>        Start date (YYYY-MM-DD) for "cf db stats".
  --to <date>          End date (YYYY-MM-DD) for "cf db stats".
  --status <status>    Keep only pending/open/done/tests_passed/accepted/
                       practice_N in "cf db stats".
  --accepted           Only count accepted problems in "cf db stats".
  --refresh            Refresh contest metadata in "cf db sync".
  --write              Allow writes in "cf db sql".
  -j, --json           Print JSON in "cf db".
  --difficulty <range> Problem rating for "cf opractice": "1200" or "800-1200".
  --tags <tags>        Comma-separated Codeforces tags for "cf opractice".
  <count>              Number of problems in the practice set.
  <name>               Practice folder name. Default "<timestamp>-practice".

Examples:
  cf config            Configure the cf-tool.
  cf submit            cf will detect what you want to submit automatically.
  cf submit -f a.cpp
  cf submit https://codeforces.com/contest/100/A
  cf submit -f a.cpp 100A 
  cf submit -f a.cpp 100 a
  cf submit contest 100 a
  cf submit gym 100001 a
  cf submit 1136a      Print the link right after submitting, and when the
                       verdict is not accepted, print the failed tests and
                       save them as inK.txt/ansK.txt for "cf test".
  cf list              List all problems' stats of a contest.
  cf list 1119
  cf ncontests         List the 20 most recent finished contests with their
                       link, division and name.
  cf parse 100         Fetch all problems' samples of contest 100 into
                       "{cf}/{contest}/100/<problem-id>".
  cf parse gym 100001a
                       Fetch samples of problem "a" of gym 100001 into
                       "{cf}/{gym}/100001/a".
  cf parse gym 100001
                       Fetch all problems' samples of gym 100001 into
                       "{cf}/{gym}/100001".
  cf parse             Fetch samples of current problem into current path.
  cf gen               Generate a code from default template and create an
                       empty analysis.md next to it.
  cf gen cpp           Generate a code from the template whose alias is "cpp"
                       into current path.
  cf test              Run the commands of a template in current path. Then
                       test all samples. If you want to add a new testcase,
                       create two files "inK.txt" and "ansK.txt" where K is
                       a string with 0~9.
  cf test -f a.cpp -d ./1a  Test a.cpp using sample files from directory ./1a.
  cf watch             Watch the first 10 submissions of current contest.
  cf watch all         Watch all submissions of current contest.
  cf open 1136a        Use default web browser to open the page of contest
                       1136, problem a.
  cf open gym 100136   Use default web browser to open the page of gym
                       100136.
  cf stand             Use default web browser to open the standing page.
  cf sid 52531875      Use default web browser to open the submission
                       52531875's page.
  cf sid               Open the last submission's page.
  cf detail https://codeforces.com/contest/1136/submission/52531875
                       Print the summary and the per-test results of a
                       submission, including input/output of failed tests.
  cf detail            Print the details of the last submission.
  cf race 1136         If the contest 1136 has not started yet, it will
                       countdown. When the countdown ends, it will open all
                       problems' pages and parse samples.
  cf pull 100          Pull all your submissions of contest 100 into
                       "./<problem-id>/<problem-id>.<ext>".
  cf pull 100 a        Pull your submissions of problem "a" of contest 100
                       into "./a.<ext>".
  cf pull ac 100 a     Pull the "Accepted" or "Pretests passed" code of problem
                       "a" of contest 100.
  cf pull              Pull your submissions of the current problem into the
                       current path.
  cf sync 1136         Sync all problems of contest 1136: fetch samples and
                       statements, pull the latest submission code, or
                       generate a code from the default template when there is
                       no submission and no local code.
  cf sync 1136a        Sync problem "a" of contest 1136.
  cf sync              Sync the current problem.
  cf                   Open the interactive shell. A timer status line is
                       refreshed every second at the bottom. There is no
                       background daemon: the state lives in
                       "~/.cf/timer.json" and "cf start"/"cf end" work from any
                       terminal. Type any cf command without the "cf" prefix,
                       for example "submit 1136a"; it runs below the timer and
                       the shell stays open. Tab completes commands and
                       arguments; Up/Down recall the command history and
                       Ctrl-R searches it. "q" closes the timer and leaves the
                       shell; Ctrl-C or Ctrl-D leaves the timer running.
                       "cf --help" prints this help.
  cf start 1136a "read the problem"
                       Start timing problem "a" of contest 1136. "cf start"
                       and "cf end" work from any terminal.
  cf end 1136a "solved"
                       End the running problem. It must match the start.
  cf start p_drill 1136a "practice"
                       Start a practice run for problem 1136a in the set
                       "{cf}/practice/drill". Accepted submits in this run
                       advance the practice label. "p_drill_1136a" can be
                       used instead of "p_drill 1136a".
  s 1136a "read"       (inside cf) Start timing without the "cf" prefix:
                       s/start and e/end; p/pause, q and status are also
                       available inside the shell.
  p                    (inside cf) Pause or resume the timer.
  status               (inside cf) Print the elapsed time, active problem and
                       state.
  q "contest done"     (inside cf) Close the timer, print the summary and
                       leave the shell. "exit" is an alias.
  submit 1136a         (inside cf) Run "cf submit 1136a" without leaving the
                       shell. Every other cf command works this way.
  cf submit p_drill_1136a
                       Submit the code of practice problem 1136a in
                       "{cf}/practice/drill". "p_<set>_<problem>" works
                       wherever a specifier is accepted, and starts a
                       practice run in "cf start/end".
  cf db sync           Index every "count_time.json" into "~/.cf/timer.db"
                       and fill contest metadata from the Codeforces API.
  cf db time 1136a     Print the time nodes and total time of a problem.
  cf db time 1136      Print per-problem totals, sum and span of a contest.
  cf db stats --from 2026-01-01 --to 2026-06-30
                       Count problems completed in a date range.
  cf db stats --accepted --from 2026-01-01
                       Count only completed problems with an accepted submit.
  cf db stats --status done --from 2026-01-01
                       Count only problems whose derived status is "done".
  cf db sql "SELECT type, COUNT(*) FROM event GROUP BY type"
                       Run a read-only SQL query. Add --write to modify data.
  cf lpractice 5       Pick 5 random problems from the local timer database.
  cf lrepractice 5     Pick 5 random accepted problems from the local database.
  cf lnrepractice 5    Pick 5 random problems never timed locally.
  cf opractice 5 --difficulty 1200-1600 --tags dp,greedy
                       Pick 5 random Codeforces problemset problems.
  cf lpractice 5 drill Create "cf/practice/drill" instead of a timestamped
                       folder.
  cf clone xalanq      Clone all codes of xalanq.
  cf statement         Fetch problem statement of current problem as statement.md.
  cf statement 1136a   Fetch statement of contest 1136 problem a.
  cf statement 1136    Fetch all statements of contest 1136.
  cf material 1136     Fetch all Contest materials for contest 1136 and save as .md files.
  cf completion bash   Print a bash completion script. Load it with
                       "source <(cf completion bash)".
  cf completion zsh    Print a zsh completion script. Load it with
                       "eval \"$(cf completion zsh)\"" in ~/.zshrc.
  cf completion fish   Print a fish completion script. Save it to
                       "~/.config/fish/completions/cf.fish".
  cf upgrade           Upgrade the "cf" to the latest version from GitHub.

File:
  cf will save some data in some files:

  "~/.cf/config"        Configuration file, including templates, etc.
  "~/.cf/session"       Session file, including cookies, handle, password, etc.
  "~/.cf/timer.json"     State of the global timer.
  "~/.cf/timer.db"       Local SQLite index of the timer records.
  "~/.cf/history"        Command history of the interactive shell.
  "<problem>/count_time.json"
                        Timer data written by "cf start" and "cf end".
  "{cf}/practice/<name>"
                        Practice sets created by "cf lpractice" and "cf opractice".

  "~" is the home directory of current user in your system.

Template:
  You can insert some placeholders into your template code. When generate a code
  from the template, cf will replace all placeholders by following rules:

  $%U%$   Handle (e.g. xalanq)
  $%Y%$   Year   (e.g. 2019)
  $%M%$   Month  (e.g. 04)
  $%D%$   Day    (e.g. 09)
  $%h%$   Hour   (e.g. 08)
  $%m%$   Minute (e.g. 05)
  $%s%$   Second (e.g. 00)

Script in template:
  Template will run 3 scripts in sequence when you run "cf test":
    - before_script   (execute once)
    - script          (execute the number of samples times)
    - after_script    (execute once)
  You could set "before_script" or "after_script" to empty string, meaning
  not executing.
  You have to run your program in "script" with standard input/output (no
  need to redirect).

  You can insert some placeholders in your scripts. When execute a script,
  cf will replace all placeholders by following rules:

  $%path%$   Path to source file (Excluding $%full%$, e.g. "/home/xalanq/")
  $%full%$   Full name of source file (e.g. "a.cpp")
  $%file%$   Name of source file (Excluding suffix, e.g. "a")
  $%rand%$   Random string with 8 character (including "a-z" "0-9")`
	color.Output = ansi.NewAnsiStdout()

	usage = strings.Replace(usage, `$%version%$`, version, 1)
	opts, _ := docopt.ParseArgs(usage, os.Args[1:], fmt.Sprintf("Codeforces Tool (cf) %v", version))
	opts[`{version}`] = version

	cfgPath, _ := homedir.Expand(configPath)
	clnPath, _ := homedir.Expand(sessionPath)
	config.Init(cfgPath)
	client.Init(clnPath, config.Instance.Host, config.Instance.Proxy)
	client.SetUserAgent(config.Instance.UserAgent)

	if len(os.Args) == 1 {
		if err := cmd.Repl(); err != nil {
			color.Red(err.Error())
		}
		color.Unset()
		return
	}

	err := cmd.Eval(opts)
	if err != nil {
		color.Red(err.Error())
	}
	color.Unset()
}
