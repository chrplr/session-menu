# session-menu

A terminal menu to run the programs of a session, one after the other, with the same subject ID and
the same arguments given to each of them. It works on Linux, Windows and macOS.

```
session-menu [-dir DIR] [SESSION_FILE]
```

It started as a copy of `dbp-meg-menu`, the menu of the
[DBP_MEG](https://github.com/digital-brain-project/DBP_MEG) experiments, made independent of them.

## The project folder

The menu works in a project folder: the current directory, or `DIR` with `-dir DIR`. It may hold:

| File or folder | Meaning |
|---|---|
| `bin/` | Programs, looked up before the PATH |
| `command_line_args.txt` | Arguments given to every program (optional; see below) |
| `.last_subject.txt` | The last subject ID, written by the menu |

The session files can be anywhere, e.g. in `sessions/`:

```
cd my-experiment
session-menu sessions/sub-001_sess-01.txt
```

## Session files

One command per line: a program, then its options. Blank lines and lines starting with `#` are
ignored. The options are split as in a shell: quotes and backslashes work. A program may be listed
several times.

```
# sub-001, session 1
!track
MEG-left-right
MEG-ARC -run 1
MEG-ARC -run 2
!python3 tasks/questionnaire.py --subject {subject}
```

- Each program is looked up first in `bin/`, then in the PATH.
- A line starting with `!` is run as written: it does not get the arguments of
  `command_line_args.txt`. Use it for programs that would refuse them, such as `track` above.
- If the file name starts with `sub-<ID>` (up to the first `_` or `.`), the Subject ID field is
  pre-filled with `<ID>`, as written (`sub-001` gives `001`).

Without a session file, the menu lists every program of `bin/`, in alphabetical order.

## Arguments common to every program

The words of `command_line_args.txt`, if the file exists, are inserted **right after the program** of
every line, except the lines that start with `!`. They are split as the session lines, may span
several lines, and lines starting with `#` are ignored.

`{subject}` is replaced by the Subject ID, in this file and in the session lines (`!` lines
included).

For example, with

```
# command_line_args.txt on the MEG PC
-s {subject}
-d 1 -ttl parallel:/dev/parport1
```

and the Subject ID 7, the session above runs (the menu prints the full paths of the programs):

```
track
bin/MEG-left-right -s 7 -d 1 -ttl parallel:/dev/parport1
bin/MEG-ARC -s 7 -d 1 -ttl parallel:/dev/parport1 -run 1
bin/MEG-ARC -s 7 -d 1 -ttl parallel:/dev/parport1 -run 2
/usr/bin/python3 tasks/questionnaire.py --subject 7
```

Because the options of a line come after the common arguments, a program that keeps the last value
of an option (as Go's `flag` package does) takes the line's value when an option is in both.

Each PC can keep its own `command_line_args.txt`, with its monitor, its trigger device and so on,
while the session files are shared.

## Use

- Type the subject ID in the *Subject ID* field: anything without blanks. Press TAB to move to the
  menu, and TAB again to come back to the field. Enter in the field also moves to the menu.
- Select an entry with the arrow keys and press Enter to run it. The menu leaves the screen while the
  program runs, so its output goes to the terminal, and comes back when it ends.
- The command line is printed before it runs.
- When a program succeeds, the cursor moves to the next entry. When it fails, its exit code is shown
  and the menu waits for Enter, so that the output can be read.
- Ctrl-C stops the running program, not the menu.
- The programs also get the subject ID in the environment variable `SUBJECT`.

The header shows the time, the subject and the common arguments.

The Subject ID field starts with the ID in the name of the session file, else the value of the
environment variable `SUBJECT`, else the last subject used.

## Errors

The menu refuses to start, and says why, if:

- the session file cannot be read, or lists no program;
- a program is found in neither `bin/` nor the PATH (with the file and line);
- a line or `command_line_args.txt` has an unclosed quote (with the file and line);
- there is no session file and `bin/` holds no program.

## Options

| Option | Meaning |
|---|---|
| `-dir DIR` | The project folder (default: the current directory) |
| `-version` | Print the version and exit |

## Installing

```
go install github.com/chrplr/session-menu@latest
```

or, from a copy of this repository:

```
go build                     # session-menu (session-menu.exe on Windows)
go test ./...
```

## License

Apache License 2.0 (see [LICENSE](LICENSE) and [NOTICE](NOTICE)).
