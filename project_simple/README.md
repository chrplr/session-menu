# A simple example project

A project folder for `session-menu` with only bash and Python scripts: nothing to build, no
configuration file. For a fuller example, with a Go program and settings in `config.toml`, see
[../project](../project/README.md).

```
project_simple/
├── command_line_args.txt   arguments given to every program: -s {subject}
├── bin/
│   ├── check_setup.sh      bash: prints the subject ID
│   ├── task.py             Python: prints its options (fails with --fail)
│   └── questionnaire.sh    bash: takes the subject ID as its only argument
└── sessions/
    ├── sub-001_sess-01.txt
    └── pilot.txt           with a failing program
```

Each script prints its arguments, then waits for Enter. Without that wait, the menu would come back at
once and cover the output.

## Running it

```
cd project_simple
session-menu
```

(`go install github.com/chrplr/session-menu@latest` installs `session-menu`. From a copy of the
repository you can instead run `go run ..` in this folder.)

The menu lists the two sessions. Open `sub-001_sess-01`: the Subject ID field is pre-filled with
`001`. Press TAB to move to the list, then Enter to run each program in turn. To see what a session
would run, without running it:

```
$ session-menu -print sessions/sub-001_sess-01.txt
# subject: 001
.../project_simple/bin/check_setup.sh -s 001
.../project_simple/bin/task.py -s 001 --run 1
.../project_simple/bin/task.py -s 001 --run 2
.../project_simple/bin/questionnaire.sh 001
```

- `check_setup.sh` and `task.py` get the common arguments, `-s 001`, right after their name, then
  the options of their line. `task.py` reads them with `argparse`, `check_setup.sh` with a `case`
  loop.
- `questionnaire.sh` wants the subject ID alone, not `-s 001`: its line starts with `!`, so it runs as
  written, without the common arguments, and `{subject}` gives it the ID.
- In `pilot.txt`, `task.py --fail` exits with code 2: the menu shows the exit code and keeps the
  cursor on it.

Every run is appended to `session-menu-log.csv`, in this folder (not in git). From it, the menu marks
each entry `✓` or `✗ exit 2`, also after it is restarted.

## The scripts

They are in `bin/`, executable, with a `#!` line (`#!/usr/bin/env bash`, `#!/usr/bin/env python3`).
So the menu finds them as programs. A new script must be made executable too:

```
chmod +x bin/my_task.sh
```

A line such as `python3 bin/task.py` would get the common arguments right after `python3`, before
the script name, where Python takes them as its own options. Write such a line with `!` and give the
options yourself (`!python3 bin/task.py -s {subject}`), or put the script in `bin/` as here.

## On Windows

The menu runs the `.py` files of `bin/` with Python, but not the `.sh` files: the sessions of this
folder, which list `.sh` scripts, run on Linux and macOS only. On Windows, the menu would refuse
them, saying that `check_setup.sh` is not executable there. `task.py` works as is; write the bash
scripts as Python scripts or `.bat` files instead (see the main [README](../README.md#session-files)).
