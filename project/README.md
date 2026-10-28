# An example project

A project folder for `session-menu`, with dummy programs: two Python scripts and a Go program. Each
reads the project's settings, prints a message and its arguments, then waits for Enter. Without that
wait, the menu would come back at once and cover the output.

```
project/
├── command_line_args.txt   arguments given to every program: -s {subject} -screen 1
├── config.toml             settings of the tasks
├── bin/
│   ├── task_a.py           Python task A
│   ├── task_b.py           Python task B (fails with --fail)
│   └── go-task             Go task, built from go-task/ (not in git)
├── go-task/                its source, a separate Go module (it reads TOML with go-toml)
└── sessions/
    ├── sub-001_sess-01.txt
    ├── sub-002_sess-01.txt
    └── pilot.txt           with '!' lines and a failing program
```

## Running it

Build the Go task into `bin/`, then start the menu from the project folder:

```
cd project
go build -C go-task -o ../bin/ .
session-menu
```

(`go install github.com/chrplr/session-menu@latest` installs `session-menu`. From a copy of the
repository you can instead run `go run ..` in this folder.)

The menu lists the three sessions. Open `sub-001_sess-01`: the Subject ID field is pre-filled with
`001`. Press TAB to move to the list, then Enter to run each program in turn. For example, the second
line of `sub-001_sess-01.txt` runs:

```
.../project/bin/go-task -s 001 -screen 1 -run 1
Hello from go-task (Example lab, screen at 60 cm)
arguments: ["-s" "001" "-screen" "1" "-run" "1"]
subject "001", screen 1, run 1
SUBJECT in the environment: 001
Press Enter to return to the menu...
```

`sessions/pilot.txt` shows the rest:

- `!` lines run as written, without the common arguments;
- `{subject}` in a session line;
- a program that fails: the menu shows its exit code and keeps the cursor on it.

Every run is appended to `session-menu-log.csv`, in this folder: when it started and ended, its exit
code, the subject, the session file, its line, and the command. The file is not in git. From it, the
menu marks each entry `✓` or `✗ exit 2`, also after it is restarted. Esc goes back to the list of
sessions.

To see what a session would run, without running it:

```
session-menu -print sessions/sub-001_sess-01.txt
```

## The Python scripts

They are in `bin/`. On Linux and macOS they are executable, with a `#!/usr/bin/env python3` line; on
Windows the menu runs them with Python (see below). So the menu finds them as programs, and they get
the common arguments right after their name:

```
.../project/bin/task_a.py -s 001 -screen 1
```

A line such as `python3 bin/task_a.py` would get the common arguments right after `python3`, before
the script name, where Python takes them as its own options. Write such a line with `!` and give the
options yourself:

```
!python3 bin/task_b.py -s {subject} --practice
```

(`pilot.txt` has `!task_b.py -s {subject} --practice` instead, which works on every system.)

The scripts read `config.toml` in the parent of their own folder, with Python's `tomllib` (Python
3.11 or later). The Go task finds it in the same way, and `-config FILE` names another file. So all
of them work from whatever folder the menu is started in.

## On Windows

The menu runs the `.py` files of `bin/` with Python: the launcher `py` (as `py -3`), or `python` if
there is no `py`. So `task_a.py` and `task_b.py` work as on the other systems, and the session files
are the same. Python must be installed; the menu says so if neither `py` nor `python` is in the PATH.

Build `go-task.exe` as on the other systems.
