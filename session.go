// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

// The session files, the programs of bin/, and the command lines.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// findPrograms returns the sorted names of the programs in dir, and the path
// of the executable to run for each of them. goos selects the layout:
// windows: <name>.exe, .bat or .cmd (in that order of preference), and
// Python scripts under their full name (<name>.py, as on the other systems;
// see interpreter); others: executable files, and on darwin also <name>.app
// bundles.
func findPrograms(dir, goos string) ([]string, map[string]string) {
	paths := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, paths
	}
	for _, e := range entries {
		name, path := e.Name(), filepath.Join(dir, e.Name())
		switch {
		case goos == "windows" && strings.EqualFold(filepath.Ext(name), ".py"):
			if !isFile(path) {
				continue
			}
		case goos == "windows":
			ext := strings.ToLower(filepath.Ext(name))
			rank := slices.Index(windowsExts, ext)
			if rank < 0 || !isFile(path) {
				continue
			}
			name = name[:len(name)-len(ext)]
			if prev, ok := paths[name]; ok && slices.Index(windowsExts, strings.ToLower(filepath.Ext(prev))) < rank {
				continue
			}
		case goos == "darwin" && strings.HasSuffix(name, ".app") && e.IsDir():
			name = strings.TrimSuffix(name, ".app")
			path = filepath.Join(path, "Contents", "MacOS", name)
			if !isExecutable(path) {
				continue
			}
		default:
			if !isExecutable(path) {
				continue
			}
		}
		paths[name] = path
	}
	names := make([]string, 0, len(paths))
	for name := range paths {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, paths
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular()
}

func isExecutable(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0
}

type entry struct {
	line   string // as listed in the menu
	lineno int    // in the session file; 0 without one
	prog   string
	args   []string
	path   string   // executable to run
	interp []string // what runs it, before it: Python for a .py on Windows
	// the line started with '!': run as written, without the common arguments
	raw bool
	// A run of a shell script: the script (nil otherwise), and how many lines
	// of it the run takes, from lineno.
	script *shellScript
	nlines int
}

// shellScript is a session file that is a shell script (see readScript).
type shellScript struct {
	path   string
	shell  string   // the executable that runs it
	lines  []string // all of its lines
	marker int      // the index of the line "# runs"
}

// runsMarker matches the comment before the runs of a script: "# runs", or
// "# Runs, one per line", but not "# runs are 10 minutes".
var runsMarker = regexp.MustCompile(`(?i)^#\s*runs\s*([:,].*)?$`)

// isScript says whether the session file at path is a shell script.
func isScript(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".sh")
}

// scriptShell returns the name of the shell of a script from its first line:
// that of its #! line (#!/bin/bash or #!/usr/bin/env bash), else sh.
func scriptShell(first string) string {
	rest, ok := strings.CutPrefix(first, "#!")
	if !ok {
		return "sh"
	}
	words := strings.Fields(rest)
	if len(words) == 0 {
		return "sh"
	}
	name := filepath.Base(words[0])
	if name != "env" {
		return name
	}
	for _, w := range words[1:] {
		if !strings.HasPrefix(w, "-") && !strings.Contains(w, "=") {
			return w
		}
	}
	return "sh"
}

// runProgram finds the program of a run as written: its first word, after any
// VAR=value. A word in quotes, or with $(...), does not match: such a run is
// not checked.
var runProgram = regexp.MustCompile(`^(?:[A-Za-z_]\w*=\S*\s+)*([\w./$\{\}:+-]+)(?:[\s;]|$)`)

// scriptCheckTime is how long the lines above the marker may take when a
// session is checked: more means that they hang.
var scriptCheckTime = 30 * time.Second

// checkPrograms runs the lines above the marker in folder dir, as a run would,
// then looks for the program of each run, with the shell's own command -v. So
// a PATH or a variable set in those lines counts, and so do the shell's
// builtins, unlike with a lookup from Go. The lines above the marker failing
// is an error too, since every run would fail in the same way. Only the first
// word of each run is checked: "uv run task" needs uv, and task is not looked
// for. A program made by an earlier run would be refused.
func (s *shellScript) checkPrograms(runs []entry, dir string) error {
	const missing = 97
	lines := slices.Clone(s.lines[:s.marker])
	for _, e := range runs {
		if m := runProgram.FindStringSubmatch(e.line); m != nil {
			lines = append(lines, fmt.Sprintf(
				`p=$(command -v -- "%s") && { case "$p" in */*) [ -x "$p" ];; esac; } || { printf '\n%d\n'; exit %d; }`,
				m[1], e.lineno, missing))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), scriptCheckTime)
	defer cancel()
	c := exec.CommandContext(ctx, s.shell, "-c", strings.Join(lines, "\n"), s.path)
	c.Dir = dir
	c.Env = append(os.Environ(), subjectVar+"=session-menu-check") // the Subject ID may not be known yet
	c.WaitDelay = time.Second                                      // a child that keeps the output open
	out, err := c.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("%s: the lines above '# runs' did not end in %v", s.path, scriptCheckTime)
	}
	if exitCode(err) == missing {
		// The last line of the output is the line of the run, unless those
		// lines exited with this code themselves.
		lineno, _ := strconv.Atoi(text[strings.LastIndexAny(text, "\n")+1:])
		if i := slices.IndexFunc(runs, func(e entry) bool { return e.lineno == lineno }); i >= 0 {
			return fmt.Errorf("%s:%d: program %s not found, or not executable (in the PATH, from %s)",
				s.path, lineno, runProgram.FindStringSubmatch(runs[i].line)[1], dir)
		}
	}
	return fmt.Errorf("%s: the lines above '# runs' failed (exit %d): %s", s.path, exitCode(err), text)
}

// readScript returns the runs of a session file that is a shell script, which
// run in the folder dir: the lines after the marker (see runsMarker), except
// blank lines and comments; a line that ends with a backslash continues on the
// next. The menu does not interpret the script: see shellScript.text. So that
// a run that cannot work is refused now and not during the session, each is
// checked with the shell's -n, which runs nothing, then with checkPrograms.
func readScript(path, dir string, lookPath func(string) (string, error)) ([]entry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	name := scriptShell(lines[0])
	shell, err := lookPath(name)
	if err != nil {
		return nil, fmt.Errorf("%s is a shell script, but %s is not in the PATH", path, name)
	}
	marker := slices.IndexFunc(lines, func(line string) bool {
		return runsMarker.MatchString(strings.TrimSpace(line))
	})
	if marker < 0 {
		return nil, fmt.Errorf("%s: no line '# runs' above the commands to run", path)
	}
	s := &shellScript{path: path, shell: shell, lines: lines, marker: marker}
	var runs []entry
	for i := marker + 1; i < len(lines); {
		if line := strings.TrimSpace(lines[i]); line == "" || strings.HasPrefix(line, "#") {
			i++
			continue
		}
		start := i
		var parts []string
		for {
			rest, more := strings.CutSuffix(strings.TrimSpace(lines[i]), `\`)
			parts = append(parts, strings.TrimSpace(rest))
			i++
			if !more || i == len(lines) {
				break
			}
		}
		e := entry{line: strings.Join(parts, " "), lineno: start + 1, script: s, nlines: i - start}
		if out, err := exec.Command(shell, "-n", "-c", s.text(e, "x"), path).CombinedOutput(); err != nil {
			why := strings.TrimSpace(string(out))
			if why == "" {
				why = err.Error()
			}
			return nil, fmt.Errorf("%s:%d: not a whole command for %s: %s", path, e.lineno, name, why)
		}
		runs = append(runs, e)
	}
	if len(runs) == 0 {
		return nil, fmt.Errorf("%s: no command below the line '# runs'", path)
	}
	if err := s.checkPrograms(runs, dir); err != nil {
		return nil, err
	}
	return runs, nil
}

// text returns what the shell runs for the run e: the script, with the other
// runs blank. So the lines above the marker set the variables of the run, as
// when the script runs on its own, without "export", and the lines keep their
// numbers in the messages of the shell. The marker becomes a check that those
// lines kept the Subject ID: a plain SUBJECT=sub-01 would otherwise replace
// the one of the menu, and the data would go under that subject. The trace
// (set -x) shows the run with its values.
func (s *shellScript) text(e entry, subject string) string {
	lines := slices.Clone(s.lines)
	id := shellQuote(subject)
	lines[s.marker] = `[ "$SUBJECT" = ` + id + ` ] || { printf '%s\n' "session-menu: this script sets SUBJECT to $SUBJECT, ` +
		`but the Subject ID is "` + id + `". Write SUBJECT=\${SUBJECT:-$SUBJECT} in it." >&2; exit 3; }`
	first := e.lineno - 1
	for i := s.marker + 1; i < len(lines); i++ {
		if i < first || i >= first+e.nlines {
			lines[i] = ""
		}
	}
	lines[first] = "set -x; " + lines[first]
	return strings.Join(lines, "\n")
}

// readSession returns the programs listed in a session file.
// One program per line, optionally followed by options passed to it.
// Blank lines and lines starting with '#' are ignored; a line starting with
// '!' gets no common arguments. A program is looked up in available (the
// programs of bin/), then with lookPath (exec.LookPath: in the PATH). goos
// is that of findPrograms, for the error messages.
func readSession(path string, available map[string]string, programsDir string,
	lookPath func(string) (string, error), goos string) ([]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var programs []entry
	sc := bufio.NewScanner(f)
	for lineno := 1; sc.Scan(); lineno++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		e := entry{line: line, lineno: lineno}
		if rest, ok := strings.CutPrefix(line, "!"); ok {
			e.raw, line = true, strings.TrimSpace(rest)
		}
		words, err := splitArgs(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, lineno, err)
		}
		if len(words) == 0 {
			return nil, fmt.Errorf("%s:%d: no program after '!'", path, lineno)
		}
		e.prog, e.args = words[0], words[1:]
		if exe, ok := available[e.prog]; ok {
			e.path = exe
			if e.interp, err = interpreter(exe, goos, lookPath); err != nil {
				return nil, fmt.Errorf("%s:%d: %v", path, lineno, err)
			}
		} else if exe, err := lookPath(e.prog); err == nil {
			e.path = exe
		} else {
			return nil, fmt.Errorf("%s:%d: %s", path, lineno, notFound(e.prog, programsDir, goos))
		}
		programs = append(programs, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(programs) == 0 {
		return nil, fmt.Errorf("%s: no program listed", path)
	}
	return programs, nil
}

// readArgsFile returns the words of the common-arguments file: split as the
// session lines, over as many lines as needed; blank lines and lines starting
// with '#' are ignored. No file means no common arguments.
func readArgsFile(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var args []string
	for i, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		words, err := splitArgs(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %v", path, i+1, err)
		}
		args = append(args, words...)
	}
	return args, nil
}

// splitArgs splits line into words like a POSIX shell (as Python's
// shlex.split): words are separated by blanks, '...' and "..." quote, and a
// backslash escapes the next character (inside "...", only \ and ").
func splitArgs(line string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	var quote rune // 0, '\'' or '"'
	escaped := false
	for _, c := range line {
		switch {
		case escaped:
			if quote == '"' && c != '"' && c != '\\' {
				word.WriteRune('\\')
			}
			word.WriteRune(c)
			escaped = false
		case c == '\\' && quote != '\'':
			escaped, inWord = true, true
		case quote != 0:
			if c == quote {
				quote = 0
			} else {
				word.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inWord = c, true
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(c)
			inWord = true
		}
	}
	if quote != 0 {
		return nil, errors.New("no closing quotation")
	}
	if escaped {
		return nil, errors.New("no escaped character")
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}

// command returns the command line of e: its executable (after its
// interpreter, if any), the common
// arguments unless the line started with '!', then the options of the line,
// with the placeholder replaced by subject. A run of a shell script gets
// neither: the common arguments would go right after its first word, as in
// "uv -s 001 run task", and the script has $SUBJECT.
func command(e entry, common []string, subject string) []string {
	if e.script != nil {
		return []string{e.script.shell, "-c", e.script.text(e, subject), e.script.path}
	}
	cmd := append(slices.Clone(e.interp), e.path)
	if !e.raw {
		cmd = append(cmd, common...)
	}
	cmd = append(cmd, e.args...)
	for i := 1; i < len(cmd); i++ {
		cmd[i] = strings.ReplaceAll(cmd[i], placeholder, subject)
	}
	return cmd
}

// shownCommand is the command of e as the terminal, the log and -print show
// it: for a run of a shell script, its line as written, not all of the script
// that the shell gets.
func shownCommand(e entry, common []string, subject string) string {
	if e.script != nil {
		return e.line
	}
	return shellJoin(command(e, common, subject))
}

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellJoin joins words for display, quoting those that a shell would split
// or that are empty.
func shellJoin(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		if w == "" || strings.ContainsAny(w, " \t'\"\\") {
			w = shellQuote(w)
		}
		quoted[i] = w
	}
	return strings.Join(quoted, " ")
}

// subjectFromName returns the label of a session file named sub-<label>_...
// (or sub-<label>.txt), as is.
func subjectFromName(path string) (string, bool) {
	name := filepath.Base(path)
	if m := regexp.MustCompile(`^sub-([^_.]+)`).FindStringSubmatch(name); m != nil {
		return m[1], true
	}
	return "", false
}

// validSubject checks a Subject ID: not empty, and without blanks, so that it
// stays one word in the command lines.
func validSubject(id string) error {
	if id == "" {
		return errors.New("Subject ID is empty.")
	}
	if strings.ContainsAny(id, " \t") {
		return errors.New("Subject ID must not contain blanks.")
	}
	return nil
}

// windowsExts are the programs of bin/ on Windows, in order of preference
// when two have the same name.
var windowsExts = []string{".exe", ".bat", ".cmd"}

// notFound says why prog, of a session file, was not found: it is a file of
// bin/ that findPrograms does not take as a program, or it is nowhere.
func notFound(prog, programsDir, goos string) string {
	path := filepath.Join(programsDir, prog)
	switch {
	case !isFile(path):
		return fmt.Sprintf("'%s' not found in %s nor in the PATH", prog, programsDir)
	case goos == "windows":
		return fmt.Sprintf("%s is not a .exe, .bat, .cmd or .py file", path)
	default:
		return fmt.Sprintf("%s exists but is not executable (chmod +x %s)", path, path)
	}
}

// interpreter returns what runs the program of bin/ at path, before it, on
// goos: on Windows, which has no #! line, Python for a .py file (the
// launcher py, else python, found with lookPath); nothing otherwise.
func interpreter(path, goos string, lookPath func(string) (string, error)) ([]string, error) {
	if goos != "windows" || !strings.EqualFold(filepath.Ext(path), ".py") {
		return nil, nil
	}
	if py, err := lookPath("py"); err == nil {
		return []string{py, "-3"}, nil
	}
	if python, err := lookPath("python"); err == nil {
		return []string{python}, nil
	}
	return nil, fmt.Errorf("%s is a Python script, but neither py nor python is in the PATH", path)
}

// listSessions returns the session files of dir, the folder sessions/ of the
// project: its *.txt and *.sh files, sorted; none if there is no such folder.
func listSessions(dir string) []string {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !(strings.EqualFold(filepath.Ext(name), ".txt") || isScript(name)) {
			continue
		}
		if path := filepath.Join(dir, name); isFile(path) {
			files = append(files, path)
		}
	}
	return files
}

// resolveSession returns the absolute path of the session file given on the
// command line: relative to the current folder cwd if it is there, else to
// the project folder root.
func resolveSession(arg, cwd, root string) string {
	if filepath.IsAbs(arg) {
		return arg
	}
	for _, dir := range []string{cwd, root} {
		if path := filepath.Join(dir, arg); isFile(path) {
			return path
		}
	}
	return filepath.Join(cwd, arg) // missing: the error will name it
}

// sessionKey is how the log names the session file at path: relative to the
// project folder root, with '/', if it is inside it; else its absolute path.
// "" (no session file) stays "".
func sessionKey(path, root string) string {
	if path == "" {
		return ""
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(rel)
}

// printCommands writes the command lines of entries for subject, one per
// line, after a comment that gives the subject.
func printCommands(w io.Writer, entries []entry, common []string, subject string) error {
	if _, err := fmt.Fprintf(w, "# subject: %s\n", subject); err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := fmt.Fprintln(w, shownCommand(e, common, subject)); err != nil {
			return err
		}
	}
	return nil
}
