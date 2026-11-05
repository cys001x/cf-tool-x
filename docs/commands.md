# Command Reference

`cf` is a command-line client for [Codeforces](https://codeforces.com). This
document describes every command, its arguments, and the files it creates.

- [Before you start](#before-you-start)
- [Specifiers](#specifiers)
- [Directory layout](#directory-layout)
- [Commands](#commands)
  - [cf config](#cf-config)
  - [cf sync](#cf-sync)
  - [cf timer](#cf-timer)
  - [cf db](#cf-db)
  - [cf practice commands](#cf-practice-commands)
  - [cf pull](#cf-pull-ac)
  - [cf clone](#cf-clone)
  - [cf parse](#cf-parse)
  - [cf statement](#cf-statement)
  - [cf material](#cf-material)
  - [cf gen](#cf-gen)
  - [cf test](#cf-test)
  - [cf submit](#cf-submit)
  - [cf watch](#cf-watch)
  - [cf detail](#cf-detail)
  - [cf list](#cf-list)
  - [cf ncontests](#cf-ncontests)
  - [cf open / stand / sid](#cf-open--stand--sid)
  - [cf race](#cf-race)
  - [cf upgrade](#cf-upgrade)
  - [cf completion](#cf-completion)
- [Environment variables](#environment-variables)
- [Troubleshooting](#troubleshooting)

## Before you start

Run `cf config` once to log in and add at least one code template. Codeforces
protects parts of the site with Cloudflare. The recommended login method is
option `1) login with browser cookies (Cloudflare)`: log in with a normal
browser, export the cookies, and paste them into `cf`. The `cf_clearance`
cookie is bound to the browser User-Agent, so also set option
`9) set browser User-Agent` to the value of `navigator.userAgent` on
codeforces.com.

Commands that download source code (`sync`, `pull`, `clone`, `detail`) need a
valid session. If they start reporting a Cloudflare challenge page, refresh
the cookies and the User-Agent with `cf config`.

## Specifiers

Most commands accept one or more `<specifier>` arguments. They can be combined;
`cf submit -f a.cpp 1136 a` is equivalent to `cf submit -f a.cpp 1136a`.

| Specifier | Meaning |
|-----------|---------|
| `1136` | Contest 1136 |
| `1136a`, `1136A` | Problem A of contest 1136 |
| `gym 100001`, `100001a` | Gym contest / gym problem |
| `Cw4JRyRGXR` | Group id (10 characters) |
| `https://codeforces.com/contest/1136` | Contest URL |
| `https://codeforces.com/contest/1136/problem/A` | Problem URL |
| `https://codeforces.com/gym/100001/problem/A` | Gym problem URL |
| `https://codeforces.com/group/Cw4JRyRGXR/contest/269760` | Group contest URL |
| `https://codeforces.com/contest/1136/submission/52531875` | Submission URL |

When no specifier is given, `cf` derives the contest and problem from the
current directory if it is inside the `cf` folder tree.

## Directory layout

Samples, statements, and generated code are stored under the configured root
folder (default `cf`) using this layout:

```
cf/
└── contest/            # or gym/, group/, acmsguru/
    └── 1136/
        └── a/
            ├── in1.txt
            ├── ans1.txt
            ├── statement.md
            └── a.cpp
```

The root folder and the per-type folder names can be changed with
`cf config` → `8) set folders' name`. The tool walks up from the current
directory until it finds the root folder, so commands work from any problem
subdirectory.

## Commands

### cf config

```
cf config
```

Interactive menu:

```
0) login                              handle/password login
1) login with browser cookies (Cloudflare)
2) add a template
3) delete a template
4) set default template
5) run "cf gen" after "cf parse"
6) set host domain
7) set proxy
8) set folders' name
9) set browser User-Agent
```

Templates are used by `cf gen` and `cf sync` when no submission is available.
A template contains the source file path, file suffixes, and optional
`before_script` / `script` / `after_script` commands used by `cf test`.

### cf sync

```
cf sync [<specifier>...]
```

Synchronizes a contest or a single problem in one command. For every selected
problem it:

1. Fetches the samples and writes `inK.txt` / `ansK.txt`.
2. Converts the problem statement to `statement.md`.
3. Looks up your **newest** submission for the problem:
   - If local code exists, asks `(y/n)` before overwriting it.
   - If no local code exists, downloads the submission as
     `<problem-id>.<ext>` (for example `a.cpp`).
4. If you have no submission and there is no local code, generates code from
   the default template.
5. If you have no submission but local code exists, keeps the local code.

Examples:

```plain
cf sync 1136          # all problems of contest 1136
cf sync 1136a         # only problem A
cf sync gym 100001    # all problems of a gym contest
cf sync               # the problem of the current directory
```

Notes:

- The problem page is fetched once per problem and reused for both the samples
  and the statement.
- The submission list is fetched once per contest, not once per problem.
- The newest submission is taken regardless of its verdict.
- Local code is detected by the known language extensions and the suffixes of
  the configured templates. Answering `n` skips that problem.
- acmsguru problems have no submission list; only samples, statement, and
  template generation apply.
- A template must be configured before `cf sync` can generate code.

### cf timer

```
cf timer [<args>...]
cf counttime [<args>...]   # alias
```

A global singleton timer. `cf timer` opens it in the foreground and
refreshes the elapsed time on one line every second; the time is printed in
green and the line is redrawn in place with the same cursor-up refresh used by
`cf submit`. Typed commands appear after `>` on the
same line. Ctrl-C or Ctrl-D leaves the monitor and keeps the timer state
active. There is no background daemon: the clock is derived from the timestamps
in `~/.cf/timer.json`, so the timer keeps counting even when no `cf` process is
running. Run `cf timer` again to see the clock, or use any command from another
terminal.

While it is open, these commands can be typed in the timer window or run from
any other terminal:

| Command | Effect |
|---------|--------|
| `start <specifier> <note>` (`s`) | Start timing a problem or a contest. Several problems can run at the same time; only a duplicate start of the same problem is rejected. The timer is created automatically when it does not exist. Prefix the specifier with `p_<set>` to record a practice run inside `{cf}/practice/<set>`, for example `cf timer s p_drill 1136a "retry"`. |
| `end <specifier> <note>` (`e`) | Stop the running problem that matches the specifier. The specifier must resolve to the same folder as one of the open `start` events; otherwise the command is rejected. For a practice run use the same `p_<set>` prefix. The note is stored as that problem's summary. The elapsed time of that problem and the timer total are printed. |
| `pause` (`p`) | Pause the timer. Running `pause` again resumes it. |
| `q [<note>]` (`exit` is an alias) | Close the timer and print the summary. The note is stored as the session summary and as the summary of every open problem. A running problem is ended automatically with the note, or `auto end on quit`. |
| `status` | Print the current elapsed time, active problem and state. |

```plain
cf timer                                  # foreground window
cf timer start 1136a "read the problem"   # from any terminal
cf timer end 1136a "solved"
cf timer pause
cf timer q "contest done"
cf timer s p_drill 1136a "practice run"
cf timer e p_drill 1136a "still wrong"
```

Automatically recorded nodes:

- `cf test` appends a `test` node when every sample passes.
- `cf submit` appends a `submit` node with the verdict. When the verdict is
  `Accepted`, `cf` asks `Accepted. Close the timer? (y/n)`. Answering `y` asks
  for a summary of the session, then closes the timer and prints the summary.

Files:

- `~/.cf/timer.json` — singleton timer state: session start, paused flag,
  accumulated elapsed time, active problem, and the list of touched records.
- `~/.cf/timer.db` — SQLite index of the records, maintained by `cf db`; see
  [cf db](#cf-db).
- `<problem>/count_time.json` — time nodes of that problem:

```json
{
  "problem": "1136a",
  "link": "https://codeforces.com/contest/1136/problem/a",
  "summary": "solved with a segment tree",
  "events": [
    {
      "type": "start",
      "time": "2026-11-05T12:00:00+08:00",
      "elapsed": "00:00:00",
      "note": "read the problem"
    },
    {
      "type": "test",
      "time": "2026-11-05T12:05:00+08:00",
      "elapsed": "00:05:00",
      "note": "all samples passed"
    }
  ]
}
```

Event types are `start`, `end`, `test` and `submit`. Every event carries the
absolute timestamp and `elapsed` in `HH:MM:SS`, measured from the timer session
start and excluding paused periods. Events recorded during a `p_<set>` timer
run also carry `practice` with the set name. `end` events also carry
`duration_ms`, the length of that timing segment. `submit` events carry
`verdict` when it is known. The record-level `summary` is the note of the last
`end` for that problem, or the note given to `q`. The session summary is
also stored in `~/.cf/timer.json`.

Notes:

- Several problems can be timed at once. The timer keeps one open set of `start` events; `test`/`submit` nodes are appended to the open problem that matches the current folder, and `end` closes the matching one.
- `start`/`end` accept the usual specifiers (`1136a`, a problem URL, a contest
  id, a gym/group URL).
- Updates from different processes are serialized with a lock file next to the
  state file.
- The monitor window only displays the timer. Closing the window keeps the
  timer state active until `cf timer q`.
- No process stays alive in the background. Every command is a short-lived
  process that reads and updates `~/.cf/timer.json`; elapsed time is computed
  from timestamps when the state is read.

### cf db

```
cf db sql [-j] [--write] <SQL>
cf db time [-j] [<specifier>...]
cf db stats [-j] [--from <date>] [--to <date>] [--accepted] [<specifier>...]
cf db sync [--refresh] [<specifier>...]
cf db path
```

A local SQLite index of the timer data. The `count_time.json` files remain the
source of truth; the database is derived from them. Every timer change also
writes through to the database, and `cf db sync` rebuilds or completes it.

`cf db time` and `cf db stats` require the database to exist. If a record was
written before the database was added, run `cf db sync` once.

Commands:

| Command | Effect |
|---------|--------|
| `sql <SQL>` | Run one SQL statement. The connection is read-only unless `--write` is given. |
| `time [<specifier>...]` | Print the time nodes and total time of a problem, or the per-problem totals, sum, and span of a contest. Without a specifier the current problem is used. |
| `stats` | Count the problems that have an `end` event in the date range, with a per-day breakdown and the total time. `--accepted` keeps only problems with an accepted submit. |
| `sync` | Index every reachable `count_time.json` and fetch missing contest metadata. Reachable means the records referenced by the timer state, the `cf` folder tree around the current directory, and any given specifiers. `--refresh` fetches metadata again. |
| `path` | Print the database path. |

```plain
cf db sync
cf db time 1136a
cf db time 1136
cf db stats --from 2026-01-01 --to 2026-06-30
cf db stats --accepted --from 2026-01-01
cf db stats --status done --from 2026-01-01
cf db sql "SELECT type, COUNT(*) FROM event GROUP BY type"
cf db sql --write "UPDATE problem SET summary = 'reviewed' WHERE key = 'contest:1136:a'"
```

Flags:

- `-j`, `--json` prints JSON instead of a table.
- `--from` and `--to` use `YYYY-MM-DD` in the local time zone. Both ends are
  inclusive; a missing `--from` means the beginning of time, a missing `--to`
  means no upper bound.
- `--accepted` changes the `stats` filter: the problem must also have a
  `submit` event with verdict `OK`.
- `--status` keeps only problems in the given state: `pending`, `open`, `done`,
  `tests_passed`, `accepted`, or `practice_1` through `practice_9`.
- `--write` allows `INSERT`, `UPDATE`, `DELETE`, and DDL in `cf db sql`.
  Without it the connection is opened read-only.
- `--refresh` re-fetches contest metadata during `cf db sync`.

Database schema:

| Table | Columns |
|-------|---------|
| `contest` | key, type (contest/gym/group/acmsguru), contest_id, group_id, name, div, start_at, end_at, link, updated_at |
| `problem` | key, contest_key, problem_id, display, name, link, dir, summary, status, practice |
| `event` | problem_key, type (start/end/test/submit), at_ms, elapsed_ms, duration_ms, verdict, note, practice |

Keys are canonical: `contest:1136:a`, `gym:100001:b`,
`group:Cw4JRyRGXR:269760:a`, and `acmsguru:99999:100`. Two views are provided:
`problem_time` (total_ms, event_count, first_start_ms, last_end_ms) and
`contest_time` (total_ms, problem_count).

Total time is the sum of `duration_ms` over `end` events. `duration_ms` is the
time between the matching `start` and `end`, excluding paused periods. For a
contest, `Sum` adds the problem times; `Span` is the wall-clock time from the
first start to the last end. With parallel problems the sum can be larger than
the span.

Every problem has one derived status label:

- `accepted`: at least one `submit` with verdict `OK`, and no counted
  practice submits.
- `practice_1` through `practice_9`: accepted submits made during a practice
  timing run (`cf timer s p_<set> ...`), excluding the first accepted submit.
  The number is the practice-accepted count, capped at 9.
- `tests_passed`: at least one `test` event exists. `cf test` only writes that
  event when every sample passes.
- `open`: the last `start` has no matching `end`.
- `done`: there is an `end` and no higher state applies.
- `pending`: no `start`, `end`, or `test` event.

The precedence is `practice_N` > `accepted` > `tests_passed` > `open` >
`done` > `pending`.

`cf db sync` fetches contest names, divisions, schedules, and problem names
from the public Codeforces API (`contest.standings`). The API does not expose
group contests or acmsguru, so those rows keep empty metadata. Metadata calls
are rate-limited to about one request per 600 ms.

Files:

- `~/.cf/timer.db` — SQLite database. Override it with `CF_TIMER_DB`.
- `~/.cf/timer.db-wal`, `~/.cf/timer.db-shm` — SQLite WAL files. They are
  created temporarily while the database is in use.

### cf practice commands

```
cf lpractice [options] <count> [<name>]
cf lrepractice [options] <count> [<name>]
cf lnrepractice [options] <count> [<name>]
cf opractice [options] <count> [<name>]
```

Create a randomized practice set. Picked problems are materialized as folders
containing samples, `statement.md`, an optional code file from the default
template, and a shared `practice.json` manifest.

| Command | Source | Selection |
|---------|--------|-----------|
| `lpractice` | local timer database | any problem |
| `lrepractice` | local timer database | `status = accepted` or `practice_N` |
| `lnrepractice` | local timer database | `status = pending`, meaning never timed or tested |
| `opractice` | Codeforces problemset API | rating and tags filters |

Local commands read `~/.cf/timer.db`, so run `cf db sync` first. They do not
accept difficulty or tag filters because the timer database does not store
rating/tag data.

`opractice` fetches `/api/problemset.problems` and filters by:

- `--difficulty <range>`: a single rating (`1200`) or an inclusive range
  (`800-1200`, `800..1200`). Without it, every rating is allowed.
- `--tags <tags>`: comma- or space-separated tags. A problem matches when it
  has at least one of the requested tags.

The selection is shuffled and truncated to `<count>`. If fewer problems match,
all of them are used and a warning is printed.

Output layout:

```
cf/
├── contest/
└── practice/
    └── 20261105-153000-practice/   # default name
        ├── practice.json
        ├── 1136a/
        │   ├── in1.txt
        │   ├── ans1.txt
        │   ├── statement.md
        │   └── 1136a.cpp
        └── 1136c/
```

- The parent is always `<root>/practice`, a sibling of `contest/`, `gym/`, and
  `group/`.
- The default subfolder name is `<YYYYMMDD-HHMMSS>-practice`. Pass `<name>` to
  choose another one. Unsafe characters are replaced with `_`.
- Each problem folder is named after `<contest-id><problem-index>`, for example
  `1136a`.
- Samples require a valid Codeforces session. Statement failures are reported
  but do not abort the set.
- A code file is generated only when a default template exists. An empty
  `analysis.md` is created next to it if missing.
- To record a practice run, start the timer from the set with
  `cf timer s p_<set> <problem>`. Only accepted submits made during that timer
  segment advance the problem's `practice` count and status.

```plain
cf lpractice 5
cf lrepractice 3 review
cf lnrepractice 10
cf opractice 5 --difficulty 1200-1600 --tags dp,greedy
cf opractice 3 --tags implementation --difficulty 800
```

### cf pull [ac]

```
cf pull [ac] [<specifier>...]
```

Downloads the source code of your submissions into the current directory.
Without a specifier it uses the current contest/problem; `ac` keeps only
`Accepted` or `Pretests passed` submissions.

```plain
cf pull 100           # all your submissions of contest 100
cf pull 100 a         # submissions of problem 100a
cf pull ac 100 a      # only accepted ones
cf pull               # current problem
```

With a problem specifier the file is written as `./<problem-id>.<ext>`.
With a contest specifier every problem gets its own folder:
`./a/a.cpp`, `./b/b.cpp`, and so on. When a file already exists, a numeric
suffix is added (`a.cpp`, `a_1.cpp`, `a_2.cpp`, ...), so repeated runs do not
overwrite anything.

This command is similar to `cf sync`, but it copies every matching submission
instead of the newest one, and it does not fetch samples or statements.

### cf clone

```
cf clone [ac] [<handle>]
```

Bulk-downloads all submissions of a Codeforces user. `handle` defaults to the
logged-in account; `ac` keeps only submissions whose API verdict is `OK`.

```plain
cf clone              # all submissions of the logged-in user
cf clone ac           # only accepted submissions
cf clone xalanq       # all submissions of another user
cf clone ac xalanq    # only accepted submissions of another user
```

The submission list comes from the public Codeforces API
(`/api/user.status`). Files are written under the current directory:

```
<handle>/
└── contest/                 # gym/ or acmsguru/ when applicable
    └── 1136/
        └── a/
            ├── 52531875.cpp
            └── 52531777_wrong_answer_3.cpp
```

- Accepted submissions use the submission id as the filename.
- Other submissions get `_<verdict>_<passed-tests>` appended.
- Existing files are skipped.
- Downloads run with 16 workers. `Too many requests` triggers a retry after
  500 seconds.
- Fetching source code still requires a valid Cloudflare session. Large
  histories can take a long time.

### cf parse

```
cf parse [<specifier>...]
```

Fetches samples only. With a contest specifier it creates one folder per
problem and downloads every sample; with a problem specifier it downloads that
problem only; without a specifier it uses the current problem.

```plain
cf parse 100
cf parse gym 100001a
cf parse
```

If `gen_after_parse` is enabled in `cf config`, a code file is also generated
from the default template.

### cf statement

```
cf statement [<specifier>...] [-d <dir>] [-f <file>]
```

Converts problem statements to markdown. For a contest it writes
`statement.md` into every problem folder; `-d` selects a single output
directory and `-f` selects the file name.

```plain
cf statement 1136
cf statement 1136a
cf statement 1136 -d ./statements -f problem.md
```

### cf material

```
cf material [<specifier>...] [-d <dir>] [-f <file>]
```

Downloads the Contest materials (announcement, tutorial, and other linked blog
entries) as markdown files.

```plain
cf material 1136
```

### cf gen

```
cf gen [<alias>]
```

Generates a source file from a configured template in the current directory.
The file is named after the current directory (the problem id). If no alias is
given, the default template is used. An empty `analysis.md` is also created in
the same directory; an existing file is kept unchanged.

```plain
cf gen
cf gen cpp
```

Placeholders such as `$%U%$` (handle), `$%Y%$` / `$%M%$` / `$%D%$`, and
`$%h%$` / `$%m%$` / `$%s%$` are replaced when the file is generated.

### cf test

```
cf test [-f <file>] [-d <dir>] [<specifier>...]
```

Runs the template's `before_script`, `script`, and `after_script` and compares
the program output with the sample answers.

- `-f` selects the source file.
- `-d` selects the directory containing `inK.txt` / `ansK.txt`.
- Without a specifier, the current directory is used.

```plain
cf test
cf test -f a.cpp -d ./1a
```

Add extra tests by creating `inK.txt` and `ansK.txt` with any string `K`
containing `0`–`9`.

### cf submit

```
cf submit [-f <file>] [<specifier>...]
```

Submits code. `-f` selects the file; without it, `cf` looks for a file whose
extension matches a configured template. The contest and problem are detected
from the current directory or from the specifier.

```plain
cf submit
cf submit -f a.cpp 1136a
```

### cf watch

```
cf watch [all] [<specifier>...]
```

Polls the current contest's submissions and updates the verdict display.
Without `all`, only the first 10 submissions are shown.

```plain
cf watch
cf watch all
```

### cf detail

```
cf detail [<specifier>...]
```

Prints the summary and per-test results of a submission, including the input,
output, and expected answer of failed tests. Per-test data is only available
for your own submissions.

```plain
cf detail https://codeforces.com/contest/1136/submission/52531875
cf detail 52531875
cf detail
```

Without a specifier, the last submission recorded by `cf` is used.

### cf list

```
cf list [<specifier>...]
```

Lists the problems of a contest with their limits and accepted counts.

```plain
cf list
cf list 1119
```

### cf ncontests

```
cf ncontests
```

Lists the 20 most recently finished Codeforces contests. Each row contains the
contest link, division, and name.

```plain
cf ncontests
```

The list comes from the public `/api/contest.list` endpoint, keeps only
finished regular contests (gym contests are excluded), and sorts them by start
time, newest first. The division is parsed from the contest name, for example
`Div. 1`, `Div. 2`, or `Div. 1 + Div. 2`; contests without a division show `-`.

### cf open / stand / sid

```plain
cf open [<specifier>...]     # open a problem or contest page
cf stand [<specifier>...]    # open the standings page
cf sid [<specifier>...]      # open a submission page
```

```plain
cf open 1136a
cf stand 1136
cf sid 52531875
```

`cf sid` without an argument opens the last submission.

### cf race

```
cf race [<specifier>...]
```

Prepares a contest: if it has not started, `cf` counts down; then it opens the
dashboard and problem pages in the browser and parses all samples.

```plain
cf race 1136
```

### cf upgrade

```
cf upgrade
```

Downloads the latest release binary from GitHub Releases and replaces the
current executable.

### cf completion

```
cf completion [<shell>]
```

Prints a shell completion script for `bash`, `zsh`, or `fish`. Without
`<shell>`, the shell is detected from `$SHELL`.

```plain
# bash: add to ~/.bashrc
$ source <(cf completion bash)

# zsh: add to ~/.zshrc
$ eval "$(cf completion zsh)"

# fish
$ cf completion fish > ~/.config/fish/completions/cf.fish
```

After reloading the shell, typing `cf sta<Tab>` completes to `cf stand`. The
scripts also complete `ac` for `pull` and `clone`, `all` for `watch`, timer
commands (`start end pause q status s e p`), `db` commands (`sql time stats
sync path`), and (in bash) file or directory paths after `-f` and `-d`.

Shell autosuggestion plugins such as zsh-autosuggestions or the fish
autosuggestion feature reuse these completion definitions to show inline hints
while typing.

## Environment variables

| Variable | Effect |
|----------|--------|
| `CF_UA` | Overrides the browser User-Agent used for every request. Takes precedence over the value stored in `~/.cf/config`. |
| `CF_TIMER_STATE` | Overrides the path of the timer state file (default `~/.cf/timer.json`). |
| `CF_TIMER_DB` | Overrides the path of the timer database (default `~/.cf/timer.db`). |

Configuration and session files:

- `~/.cf/config` — handle, templates, host, proxy, folder names, User-Agent.
- `~/.cf/session` — cookies and login session.

## Troubleshooting

**`Cloudflare returned a challenge page instead of codeforces.`**

The `cf_clearance` cookie expired or does not match the User-Agent. Run
`cf config` → `1) login with browser cookies (Cloudflare)` and set
`9) set browser User-Agent` from `navigator.userAgent` on codeforces.com.

**`Not logged in`**

Run `cf config` and log in before using commands that need a session.

**`Cannot find any code template` / `You have to add at least one code
template`**

Add a template with `cf config` → `2) add a template`. `cf gen` and `cf sync`
need one when they have to generate code.

**`Language "..." has no known file extension`**

`cf sync` could not map the submission's language to a file extension. Add the
extension manually or use `cf pull` with a supported language.
