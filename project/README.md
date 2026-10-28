# An example project

A project folder for `session-menu`, with dummy programs: two Python scripts and a Go program. Each
reads the project's settings, prints a message and its arguments, then waits for Enter. Without that
wait, the menu would come back at once and cover the output.

```
project/
├── command_line_args.txt   arguments given to every program: -s {subject} -screen 1
├── config.py               settings of the Python tasks
├── config.toml             settings of the Go task
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
session-menu sessions/sub-001_sess-01.txt
```

(`go install github.com/chrplr/session-menu@latest` installs `session-menu`. From a copy of the
repository you can instead run `go run .. sessions/sub-001_sess-01.txt` in this folder.)

The Subject ID field is pre-filled with `001`. Press TAB to move to the list, then Enter to run each
program in turn. For example, the second line of `sub-001_sess-01.txt` runs:

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
code, the subject, the session file, its line, and the command. The file is not in git.

## The Python scripts

They are in `bin/` and executable, with a `#!/usr/bin/env python3` line. So the menu finds them as
programs, and they get the common arguments right after their name:

```
.../project/bin/task_a.py -s 001 -screen 1
```

A line such as `python3 bin/task_a.py` would get the common arguments right after `python3`, before
the script name, where Python takes them as its own options. Write such a line with `!` and give the
options yourself, as in `pilot.txt`:

```
!python3 bin/task_b.py -s {subject} --practice
```

The scripts find `config.py` in the parent of their own folder. The Go task finds `config.toml` in
the same way, and `-config FILE` names another file. So both work from whatever folder the menu is
started in.

## On Windows

The menu only lists `.exe` files of `bin/` there, so `task_a.py` and `task_b.py` are not found.
Write their lines as `!python bin\task_a.py -s {subject} …`. `echo`, in `pilot.txt`, is not a
program on Windows either. `go-task.exe` works as on the other systems.
