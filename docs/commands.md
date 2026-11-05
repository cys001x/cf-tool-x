# Command Reference

`cf` is a command-line client for [Codeforces](https://codeforces.com). This
document describes every command, its arguments, and the files it creates.

- [Before you start](#before-you-start)
- [Specifiers](#specifiers)
- [Directory layout](#directory-layout)
- [Commands](#commands)
  - [cf config](#cf-config)
  - [cf sync](#cf-sync)
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
given, the default template is used.

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
scripts also complete `ac` for `pull` and `clone`, `all` for `watch`, and (in
bash) file or directory paths after `-f` and `-d`.

Shell autosuggestion plugins such as zsh-autosuggestions or the fish
autosuggestion feature reuse these completion definitions to show inline hints
while typing.

## Environment variables

| Variable | Effect |
|----------|--------|
| `CF_UA` | Overrides the browser User-Agent used for every request. Takes precedence over the value stored in `~/.cf/config`. |

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
