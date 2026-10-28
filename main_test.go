// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

import (
	"bytes"
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
	// on Windows: .bat and .cmd too, .exe first, then .bat
	write(t, filepath.Join(dir, "c-prog.bat"), "", 0o644)
	write(t, filepath.Join(dir, "f-prog.CMD"), "", 0o644)
	write(t, filepath.Join(dir, "f-prog.bat"), "", 0o644)
	write(t, filepath.Join(dir, "g-prog.cmd"), "", 0o644)
	// on Windows: Python scripts too, under their full name
	write(t, filepath.Join(dir, "h-prog.py"), "", 0o644)
	write(t, filepath.Join(dir, "d-prog.app", "Contents", "MacOS", "d-prog"), "", 0o755)
	write(t, filepath.Join(dir, "e-prog.app", "Contents", "Info.plist"), "", 0o644)

	a, b := filepath.Join(dir, "a-prog"), filepath.Join(dir, "b-prog")
	for _, tc := range []struct {
		goos  string
		names []string
		paths map[string]string
	}{
		{"linux", []string{"a-prog", "b-prog"}, map[string]string{"a-prog": a, "b-prog": b}},
		{"windows", []string{"c-prog", "f-prog", "g-prog", "h-prog.py"}, map[string]string{"c-prog": filepath.Join(dir, "c-prog.exe"),
			"f-prog": filepath.Join(dir, "f-prog.bat"), "g-prog": filepath.Join(dir, "g-prog.cmd"),
			"h-prog.py": filepath.Join(dir, "h-prog.py")}},
		// executables, and .app bundles
		{"darwin", []string{"a-prog", "b-prog", "d-prog"}, map[string]string{"a-prog": a, "b-prog": b,
			"d-prog": filepath.Join(dir, "d-prog.app", "Contents", "MacOS", "d-prog")}},
	} {
		if tc.goos != "windows" && runtime.GOOS == "windows" {
			continue // Windows files have no executable bit
		}
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
	got, err := readSession(session, available, filepath.Join(dir, "progs"), lookPath, "linux")
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

	progs := filepath.Join(dir, "progs")
	write(t, filepath.Join(progs, "task.py"), "", 0o644) // not executable
	for content, msg := range map[string]string{
		"A\nC -x\n":     ":2: 'C' not found in " + progs + " nor in the PATH",
		"task.py\n":     ":1: " + filepath.Join(progs, "task.py") + " exists but is not executable (chmod +x",
		"# nothing\n\n": "no program listed",
		"A 'unclosed\n": ":1: no closing quotation",
		"A\n!\n":        ":2: no program after '!'",
	} {
		write(t, session, content, 0o644)
		if _, err := readSession(session, available, progs, lookPath, "linux"); err == nil || !strings.Contains(err.Error(), msg) {
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

func TestNotFoundWindows(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "task.txt"), "", 0o644)
	if msg := notFound("task.txt", dir, "windows"); !strings.Contains(msg, "is not a .exe, .bat, .cmd or .py file") {
		t.Errorf("notFound = %q", msg)
	}
}

func TestListSessions(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"sub-002.txt", "pilot.TXT", "sub-001_sess-01.txt", "notes.md", ".hidden.txt"} {
		write(t, filepath.Join(dir, name), "", 0o644)
	}
	if err := os.Mkdir(filepath.Join(dir, "old.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, p := range listSessions(dir) {
		got = append(got, filepath.Base(p))
	}
	if want := []string{"pilot.TXT", "sub-001_sess-01.txt", "sub-002.txt"}; !reflect.DeepEqual(got, want) {
		t.Errorf("listSessions = %q, want %q", got, want)
	}
	if got := listSessions(filepath.Join(dir, "missing")); got != nil {
		t.Errorf("missing folder: listSessions = %q", got)
	}
}

func TestResolveSession(t *testing.T) {
	cwd, root := t.TempDir(), t.TempDir()
	write(t, filepath.Join(cwd, "here.txt"), "", 0o644)
	write(t, filepath.Join(root, "sessions", "s.txt"), "", 0o644)
	write(t, filepath.Join(cwd, "both.txt"), "", 0o644)
	write(t, filepath.Join(root, "both.txt"), "", 0o644)
	for arg, want := range map[string]string{
		"here.txt":                         filepath.Join(cwd, "here.txt"),
		filepath.Join("sessions", "s.txt"): filepath.Join(root, "sessions", "s.txt"),
		"both.txt":                         filepath.Join(cwd, "both.txt"), // the current folder first
		"missing.txt":                      filepath.Join(cwd, "missing.txt"),
		filepath.Join(root, "both.txt"):    filepath.Join(root, "both.txt"),
	} {
		if got := resolveSession(arg, cwd, root); got != want {
			t.Errorf("resolveSession(%q) = %q, want %q", arg, got, want)
		}
	}
}

func TestSessionKey(t *testing.T) {
	root := filepath.Join(t.TempDir(), "project")
	outside := filepath.Join(filepath.Dir(root), "elsewhere", "s.txt")
	for path, want := range map[string]string{
		filepath.Join(root, "sessions", "s.txt"):          "sessions/s.txt",
		filepath.Join(root, "s.txt"):                      "s.txt",
		outside:                                           outside,
		filepath.Join(filepath.Dir(root), "project2.txt"): filepath.Join(filepath.Dir(root), "project2.txt"),
		"": "",
	} {
		if got := sessionKey(path, root); got != want {
			t.Errorf("sessionKey(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestPrintCommands(t *testing.T) {
	entries := []entry{
		{path: "/x/A", args: []string{"-run", "1"}},
		{path: "/usr/bin/track", raw: true},
		{path: "/x/B", args: []string{"-title", "a b"}},
	}
	var b bytes.Buffer
	if err := printCommands(&b, entries, []string{"-s", "{subject}"}, "007"); err != nil {
		t.Fatal(err)
	}
	want := "# subject: 007\n/x/A -s 007 -run 1\n/usr/bin/track\n/x/B -s 007 -title 'a b'\n"
	if b.String() != want {
		t.Errorf("printCommands:\n%s\nwant\n%s", b.String(), want)
	}
}

func TestLoadMarks(t *testing.T) {
	path := filepath.Join(t.TempDir(), logFile)
	if marks, err := loadMarks(path, "1", "sessions/a.txt"); err != nil || len(marks) != 0 {
		t.Errorf("no log: loadMarks = %v, %v", marks, err)
	}
	t0 := time.Date(2026, 10, 28, 14, 0, 0, 0, time.UTC)
	for _, r := range []struct {
		code             int
		subject, session string
		line             int
	}{
		{0, "1", "sessions/a.txt", 3},
		{2, "1", "sessions/a.txt", 4},
		{0, "1", "sessions/a.txt", 4}, // the last run of line 4 wins
		{-1, "1", "sessions/a.txt", 5},
		{0, "2", "sessions/a.txt", 5}, // another subject
		{0, "1", "sessions/b.txt", 6}, // another session
		{0, "1", "", 0},               // a program of bin/
	} {
		if err := appendLog(path, result{t0, t0, r.code}, r.subject, r.session, r.line, []string{"/x/A"}); err != nil {
			t.Fatal(err)
		}
	}
	marks, err := loadMarks(path, "1", "sessions/a.txt")
	if want := map[int]int{3: 0, 4: 0, 5: -1}; err != nil || !reflect.DeepEqual(marks, want) {
		t.Errorf("loadMarks = %v, %v; want %v", marks, err, want)
	}
	write(t, path, "start,end\n1,2\n", 0o644)
	if _, err := loadMarks(path, "1", "sessions/a.txt"); err == nil {
		t.Error("log without the needed columns: no error")
	}
}

func TestMarkText(t *testing.T) {
	e := entry{line: "task_b.py --fail [x]"}
	for _, tc := range []struct {
		code int
		ran  bool
		want string
	}{
		{0, false, "           3. task_b.py --fail [x[]"},
		{0, true, "✓          3. task_b.py --fail [x[]"},
		{2, true, "✗ exit 2   3. task_b.py --fail [x[]"},
		{-1, true, "✗ failed   3. task_b.py --fail [x[]"},
	} {
		if got := markText(2, e, tc.code, tc.ran); got != tc.want {
			t.Errorf("markText(%d, %v) = %q, want %q", tc.code, tc.ran, got, tc.want)
		}
	}
}

func TestInterpreter(t *testing.T) {
	path := filepath.Join("bin", "task_a.py")
	inPath := func(names ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			for _, n := range names {
				if n == name {
					return "/w/" + name, nil
				}
			}
			return "", errors.New("not found")
		}
	}
	for _, tc := range []struct {
		name, path, goos string
		lookPath         func(string) (string, error)
		want             []string
	}{
		{"py launcher", path, "windows", inPath("py", "python"), []string{"/w/py", "-3"}},
		{"python only", path, "windows", inPath("python"), []string{"/w/python"}},
		{"upper-case .PY", filepath.Join("bin", "T.PY"), "windows", inPath("py"), []string{"/w/py", "-3"}},
		{"not a .py", filepath.Join("bin", "t.exe"), "windows", inPath("py"), nil},
		{"#! line elsewhere", path, "linux", inPath("py"), nil},
	} {
		got, err := interpreter(tc.path, tc.goos, tc.lookPath)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: interpreter = %q, %v; want %q", tc.name, got, err, tc.want)
		}
	}
	if _, err := interpreter(path, "windows", inPath()); err == nil || !strings.Contains(err.Error(), "neither py nor python") {
		t.Errorf("no Python: error %v", err)
	}
}

func TestReadSessionWindowsPy(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "s.txt")
	write(t, session, "task_a.py -run 1\n", 0o644)
	available := map[string]string{"task_a.py": `C:\p\bin\task_a.py`}
	lookPath := func(name string) (string, error) {
		if name == "py" {
			return `C:\Windows\py.exe`, nil
		}
		return "", errors.New("not found")
	}
	entries, err := readSession(session, available, "bin", lookPath, "windows")
	if err != nil {
		t.Fatal(err)
	}
	got := command(entries[0], []string{"-s", "{subject}"}, "001")
	want := []string{`C:\Windows\py.exe`, "-3", `C:\p\bin\task_a.py`, "-s", "001", "-run", "1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("command = %q, want %q", got, want)
	}
	// Without Python, the session does not open.
	if _, err := readSession(session, available, "bin", func(string) (string, error) { return "", errors.New("no") }, "windows"); err == nil ||
		!strings.Contains(err.Error(), ":1: ") {
		t.Errorf("no Python: error %v", err)
	}
}
