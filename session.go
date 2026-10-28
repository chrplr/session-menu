// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

// The session files, the programs of bin/, and the command lines.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// findPrograms returns the sorted names of the programs in dir, and the path
// of the executable to run for each of them. goos selects the layout:
// windows: <name>.exe, .bat or .cmd (in that order of preference); others:
// executable files, and on darwin also <name>.app bundles.
func findPrograms(dir, goos string) ([]string, map[string]string) {
	paths := map[string]string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, paths
	}
	for _, e := range entries {
		name, path := e.Name(), filepath.Join(dir, e.Name())
		switch {
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
	path   string // executable to run
	// the line started with '!': run as written, without the common arguments
	raw bool
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

// command returns the command line of e: its executable, the common
// arguments unless the line started with '!', then the options of the line,
// with the placeholder replaced by subject.
func command(e entry, common []string, subject string) []string {
	cmd := []string{e.path}
	if !e.raw {
		cmd = append(cmd, common...)
	}
	cmd = append(cmd, e.args...)
	for i := 1; i < len(cmd); i++ {
		cmd[i] = strings.ReplaceAll(cmd[i], placeholder, subject)
	}
	return cmd
}

// shellJoin joins words for display, quoting those that a shell would split
// or that are empty.
func shellJoin(words []string) string {
	quoted := make([]string, len(words))
	for i, w := range words {
		if w == "" || strings.ContainsAny(w, " \t'\"\\") {
			w = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
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
		return fmt.Sprintf("%s is not a .exe, .bat or .cmd file", path)
	default:
		return fmt.Sprintf("%s exists but is not executable (chmod +x %s)", path, path)
	}
}

// listSessions returns the session files of dir, the folder sessions/ of the
// project: its *.txt files, sorted; none if there is no such folder.
func listSessions(dir string) []string {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !strings.EqualFold(filepath.Ext(name), ".txt") {
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
		if _, err := fmt.Fprintln(w, shellJoin(command(e, common, subject))); err != nil {
			return err
		}
	}
	return nil
}
