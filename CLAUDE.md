# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

`session-menu` is a terminal menu (Go, tview/tcell) that runs the programs of an experiment session one
after the other, with the same subject ID and common arguments. `README.md` is the user documentation,
and it ships in the release archives.

## Commands

There is no Makefile and no linter configuration. CI (`.github/workflows/ci.yml`) runs:

```
go vet ./...
go test ./...
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o <outdir>/ .     # also windows/amd64, darwin/arm64
go -C project2/go-task vet ./...                                  # a separate module, see below
go -C project2/go-task build -o ../bin/ .
```

CI also runs `go test -v ./...` on a Windows runner.

Other commands:

```
go test -run TestTimerText -v .                       # one test
GOOS=windows go vet ./...                             # compiles the Windows-only tests on any system
go run . -dir project1                                # the menu on an example project
go run . -dir project1 -print sessions/sub-001_sess-01.txt   # what a session would run, without a terminal UI
./package.sh                                          # release archives in dist/; MAINTAINER.md has the release steps
```

## Architecture

One `main` package, in layers:

- `main.go`: flags, then the choice of the first page: a session file given, else the picker over
  `sessions/*.txt`, else every program of `bin/`.
- `session.go`: no UI. Finds programs, parses session files and `command_line_args.txt`, and builds
  command lines (`command`).
- `log.go`: appends to `session-menu-log.csv`, and reads the ✓/✗ marks back from it (`loadMarks`).
- `ui.go`: the tview `menu`. `runEntry` validates the Subject ID, sets `SUBJECT`, runs the program inside
  `app.Suspend`, then updates the mark and appends to the log.

Things that span several files:

- **Argument order is the contract:** executable, common arguments (skipped for `!` lines), then the
  line's options, with `{subject}` replaced everywhere. Programs that keep the last value of an option
  rely on the line's options coming last.
- **Marks are looked up by subject, session key and line number** (`lineno` in the session file).
  `sessionKey` makes the session path relative to the project folder, so marks survive moving the
  project. The programs of `bin/`, with no session file, have no line numbers: their marks last only as
  long as the menu.
- **Platform differences are parameters.** `findPrograms`, `readSession`, `notFound` and `interpreter`
  take `goos` and, where needed, `lookPath`, so their Windows and macOS behaviour is tested on any
  system. Prefer this to build tags. `exec_windows_test.go` is Windows-only through its file name, and
  it is the only test that actually runs `.bat` and `.py` programs.
- **There are no automated tests of the UI.** Keep logic in pure functions such as `markText` and
  `timerText`, and test those in `main_test.go`. To check the UI, drive the real menu under tmux
  (`send-keys`, `capture-pane`); use `pipe-pane` to see exactly what it writes to the terminal.

## tview traps

These come from how tview v0.42.0 works, so recheck them after upgrading it:

- `runEntry` runs on the event loop, and `Suspend` keeps the screen active while the program runs. From
  another goroutine, use `app.Draw()`, which waits in the event loop's queue. `ForceDraw` would write
  over the running program's output.
- `QueueUpdate` and `Draw` block until the event loop has run them, so do not call them from the event
  loop itself (key handlers, selected functions).
- `SetBeforeDrawFunc` handlers run while the application is locked, so they must not call `app` methods.
- The `guard` in `ui.go` (keys ignored for 500 ms after a program returns) is deliberate: it stops a
  double Enter from starting the next program.

## Example projects

- `project1`: bash and Python scripts, nothing to build.
- `project2`: adds a Go program and `config.toml`. `project2/go-task` is its own Go module, so the root
  `go test ./...` does not cover it; only the CI step above does.
- `project3`: shell scripts and rig JSON files. It has no `*.txt` session files, and the READMEs do not
  mention it.

Renaming an example folder also means updating the READMEs, `working-directory` in `ci.yml`, and, for
`project2`, the module path in `go-task/go.mod`.

## Conventions

- Every `.go` file starts with the copyright and Apache-2.0 header.
- A new flag goes in `flag.Usage`, the synopsis in the `main.go` package comment, and the README's
  *Options* table.
