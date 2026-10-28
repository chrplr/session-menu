// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

import (
	"encoding/csv"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestSplitArgs(t *testing.T) {
	for _, tc := range []struct {
		line string
		want []string
	}{
		{"MEG-left-right", []string{"MEG-left-right"}},
		{"MEG-left-right  -seed 2\t-w", []string{"MEG-left-right", "-seed", "2", "-w"}},
		{`prog -title 'a b' -x "c d"`, []string{"prog", "-title", "a b", "-x", "c d"}},
		{`prog a\ b "x\"y" 'x\y' "x\y"`, []string{"prog", "a b", `x"y`, `x\y`, `x\y`}},
		{`prog '' foo"bar"baz`, []string{"prog", "", "foobarbaz"}},
	} {
		got, err := splitArgs(tc.line)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("splitArgs(%q) = %q, %v; want %q", tc.line, got, err, tc.want)
		}
	}
	for _, line := range []string{`prog 'a`, `prog "a`, `prog a\`} {
		if _, err := splitArgs(line); err == nil {
			t.Errorf("splitArgs(%q): no error", line)
		}
	}
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func TestFindPrograms(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "b-prog"), "", 0o755)
	write(t, filepath.Join(dir, "a-prog"), "", 0o755)
	write(t, filepath.Join(dir, "notes.txt"), "", 0o644)
	write(t, filepath.Join(dir, "c-prog.exe"), "", 0o644)
	write(t, filepath.Join(dir, "d-prog.app", "Contents", "MacOS", "d-prog"), "", 0o755)
	write(t, filepath.Join(dir, "e-prog.app", "Contents", "Info.plist"), "", 0o644)

	a, b := filepath.Join(dir, "a-prog"), filepath.Join(dir, "b-prog")
	for _, tc := range []struct {
		goos  string
		names []string
		paths map[string]string
	}{
		{"linux", []string{"a-prog", "b-prog"}, map[string]string{"a-prog": a, "b-prog": b}},
		{"windows", []string{"c-prog"}, map[string]string{"c-prog": filepath.Join(dir, "c-prog.exe")}},
		// executables, and .app bundles
		{"darwin", []string{"a-prog", "b-prog", "d-prog"}, map[string]string{"a-prog": a, "b-prog": b,
			"d-prog": filepath.Join(dir, "d-prog.app", "Contents", "MacOS", "d-prog")}},
	} {
		names, paths := findPrograms(dir, tc.goos)
		if !reflect.DeepEqual(names, tc.names) || !reflect.DeepEqual(paths, tc.paths) {
			t.Errorf("%s: findPrograms = %q, %q; want %q, %q", tc.goos, names, paths, tc.names, tc.paths)
		}
	}

	if names, _ := findPrograms(filepath.Join(dir, "missing"), "linux"); len(names) != 0 {
		t.Errorf("missing dir: findPrograms = %q", names)
	}
}

func TestReadSession(t *testing.T) {
	dir := t.TempDir()
	available := map[string]string{"A": "/x/A", "B": "/x/B", "track": "/x/track"}
	// the PATH holds track (shadowed by bin/) and calib
	lookPath := func(name string) (string, error) {
		switch name {
		case "track", "calib":
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	}
	session := filepath.Join(dir, "sub-001_sess-01.txt")

	write(t, session, "# comment\n\nA\n  B -seed 2 \n!calib -n 9\n! track\nA\n", 0o644)
	got, err := readSession(session, available, "progs", lookPath)
	want := []entry{
		{line: "A", lineno: 3, prog: "A", args: []string{}, path: "/x/A"},
		{line: "B -seed 2", lineno: 4, prog: "B", args: []string{"-seed", "2"}, path: "/x/B"},
		{line: "!calib -n 9", lineno: 5, prog: "calib", args: []string{"-n", "9"}, path: "/usr/bin/calib", raw: true},
		{line: "! track", lineno: 6, prog: "track", args: []string{}, path: "/x/track", raw: true}, // bin/ first
		{line: "A", lineno: 7, prog: "A", args: []string{}, path: "/x/A"},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("readSession = %+v, %v; want %+v", got, err, want)
	}

	for content, msg := range map[string]string{
		"A\nC -x\n":     ":2: 'C' not found in progs nor in the PATH",
		"# nothing\n\n": "no program listed",
		"A 'unclosed\n": ":1: no closing quotation",
		"A\n!\n":        ":2: no program after '!'",
	} {
		write(t, session, content, 0o644)
		if _, err := readSession(session, available, "progs", lookPath); err == nil || !strings.Contains(err.Error(), msg) {
			t.Errorf("readSession(%q): error %v, want %q", content, err, msg)
		}
	}
}

func TestReadArgsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, argsFile)
	if args, err := readArgsFile(path); err != nil || args != nil {
		t.Errorf("no file: readArgsFile = %q, %v; want none", args, err)
	}

	write(t, path, "# common arguments\n-s {subject}\n\n  -d 1 -title 'a b'\n# -ttl null\n", 0o644)
	want := []string{"-s", "{subject}", "-d", "1", "-title", "a b"}
	if args, err := readArgsFile(path); err != nil || !reflect.DeepEqual(args, want) {
		t.Errorf("readArgsFile = %q, %v; want %q", args, err, want)
	}

	write(t, path, "-s {subject}\n-title 'a b\n", 0o644)
	if _, err := readArgsFile(path); err == nil || !strings.Contains(err.Error(), ":2: no closing quotation") {
		t.Errorf("unclosed quote: error %v", err)
	}
}

func TestCommand(t *testing.T) {
	common := []string{"-s", "{subject}", "-d", "1"}
	for _, tc := range []struct {
		name string
		e    entry
		want []string
	}{
		{"common args right after the program", entry{path: "/x/A", args: []string{"-run", "1"}},
			[]string{"/x/A", "-s", "007", "-d", "1", "-run", "1"}},
		{"no options", entry{path: "/x/A"},
			[]string{"/x/A", "-s", "007", "-d", "1"}},
		{"'!': as written", entry{path: "/usr/bin/track", raw: true},
			[]string{"/usr/bin/track"}},
		{"'!' with the placeholder", entry{path: "/usr/bin/python3", args: []string{"task.py", "--subject={subject}"}, raw: true},
			[]string{"/usr/bin/python3", "task.py", "--subject=007"}},
	} {
		if got := command(tc.e, common, "007"); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: command = %q, want %q", tc.name, got, tc.want)
		}
	}
	// The common arguments are not modified by the replacement.
	if common[1] != "{subject}" {
		t.Errorf("common args changed: %q", common)
	}
	// No common arguments file.
	if got := command(entry{path: "/x/A", args: []string{"-n", "2"}}, nil, "1"); !reflect.DeepEqual(got, []string{"/x/A", "-n", "2"}) {
		t.Errorf("no common args: command = %q", got)
	}
}

func TestShellJoin(t *testing.T) {
	got := shellJoin([]string{"/x/A", "-title", "a b", "", "it's"})
	if want := `/x/A -title 'a b' '' 'it'\''s'`; got != want {
		t.Errorf("shellJoin = %s, want %s", got, want)
	}
	// What is displayed splits back into the same words.
	words := []string{"/x/A", "-title", "a b", "", `x"y`}
	if back, err := splitArgs(shellJoin(words)); err != nil || !reflect.DeepEqual(back, words) {
		t.Errorf("splitArgs(shellJoin(%q)) = %q, %v", words, back, err)
	}
}

func TestSubject(t *testing.T) {
	for name, want := range map[string]string{
		"sessions/sub-007_sess-01.txt": "007",
		"sub-AB12.txt":                 "AB12",
		"pilot.txt":                    "",
		"my-sub-1_x.txt":               "",
	} {
		got, ok := subjectFromName(name)
		if got != want || ok != (want != "") {
			t.Errorf("subjectFromName(%q) = %q, %v; want %q", name, got, ok, want)
		}
	}
	for id, ok := range map[string]bool{"1": true, "007": true, "P01-a": true, "": false, "1 2": false, "a\tb": false} {
		if err := validSubject(id); (err == nil) != ok {
			t.Errorf("validSubject(%q) = %v", id, err)
		}
	}
}

func TestExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	for script, want := range map[string]int{"exit 0": 0, "exit 3": 3, "kill -TERM $$": -1} {
		if got := exitCode(exec.Command("sh", "-c", script).Run()); got != want {
			t.Errorf("%q: exit code %d, want %d", script, got, want)
		}
	}
	if got := exitCode(exec.Command(filepath.Join(t.TempDir(), "missing")).Run()); got != -1 {
		t.Errorf("program not found: exit code %d, want -1", got)
	}
}

func TestAppendLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), logFile)
	start := time.Date(2026, 10, 28, 14, 30, 0, 0, time.FixedZone("CET", 3600))
	runs := []struct {
		r       result
		session string
		lineno  int
		cmd     []string
	}{
		{result{start, start.Add(1500 * time.Millisecond), 0}, "sessions/sub-1.txt", 3, []string{"/x/A", "-s", "1"}},
		// a command with a comma and quotes, and no session file
		{result{start.Add(time.Minute), start.Add(time.Minute + 250*time.Millisecond), 2}, "", 0,
			[]string{"/x/B", "-title", `a, "b"`}},
	}
	for _, r := range runs {
		if err := appendLog(path, r.r, "1", r.session, r.lineno, r.cmd); err != nil {
			t.Fatal(err)
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		logHeader,
		{"2026-10-28T14:30:00.000+01:00", "2026-10-28T14:30:01.500+01:00", "1.500", "0", "1", "sessions/sub-1.txt", "3", "/x/A -s 1"},
		{"2026-10-28T14:31:00.000+01:00", "2026-10-28T14:31:00.250+01:00", "0.250", "2", "1", "", "", `/x/B -title 'a, "b"'`},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("log:\n%q\nwant\n%q", rows, want)
	}
}
