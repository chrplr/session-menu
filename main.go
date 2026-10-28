// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

// session-menu runs the programs of a session, one after the other, from a
// terminal menu:
//
//	session-menu [-dir DIR] [-print] [SESSION_FILE]
//
// A session file lists one command per line: a program, then its options. A
// program is looked up in DIR/bin, then in the PATH. Without SESSION_FILE,
// the menu lists the session files of DIR/sessions to choose from, or, if
// there are none, every program of DIR/bin.
//
// The words of DIR/command_line_args.txt, if it exists, are inserted right
// after the program of every line, except the lines that start with '!',
// which run as written. "{subject}" is replaced by the Subject ID, in that
// file and in the session lines. The programs run in DIR.
//
// Every run is appended to DIR/session-menu-log.csv: when it started and
// finished, and its exit code. The menu marks the lines already run from it.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	quit        = "Quit"
	binDir      = "bin"
	sessionsDir = "sessions"
	argsFile    = "command_line_args.txt"
	subjectFile = ".last_subject.txt"
	logFile     = "session-menu-log.csv"
	// timeFormat is that of the times of the log: local time, to the
	// millisecond, with the offset from UTC.
	timeFormat = "2006-01-02T15:04:05.000-07:00"
	// subjectVar is set to the Subject ID in the environment of the programs.
	subjectVar = "SUBJECT"
	// placeholder is replaced by the Subject ID in the command lines.
	placeholder = "{subject}"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// version is the module version (with go install ...@vX), else the VCS
// revision the binary was built from.
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	rev, modified := "", false
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	if rev == "" {
		return "(devel)"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if modified {
		rev += "-dirty"
	}
	return rev
}

func main() {
	dir := flag.String("dir", "", "project folder, with bin/, sessions/, "+argsFile+", "+subjectFile+
		" and "+logFile+"; the programs run in it (default: the current directory)")
	printOnly := flag.Bool("print", false, "print the command lines of SESSION_FILE, with the Subject ID and "+
		"the common arguments, and exit")
	showVersion := flag.Bool("version", false, "print the version of this program and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-dir DIR] [-print] [-version] [SESSION_FILE]\n\n"+
			"SESSION_FILE: file listing the commands to run, one per line "+
			"(default: choose one of DIR/sessions/*.txt, or, if there are none, all the programs of DIR/bin)\n\n",
			filepath.Base(os.Args[0]))
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("session-menu", version())
		return
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	cwd, err := os.Getwd()
	if err != nil {
		fatalf("%v", err)
	}
	root, err := filepath.Abs(*dir)
	if err != nil {
		fatalf("%v", err)
	}
	programsDir := filepath.Join(root, binDir)
	subjectPath := filepath.Join(root, subjectFile)

	common, err := readArgsFile(filepath.Join(root, argsFile))
	if err != nil {
		fatalf("%v", err)
	}

	// The Subject ID starts with: the ID in the name of the session file
	// (when it is opened), else $SUBJECT, else the last subject.
	subjectID := ""
	if b, err := os.ReadFile(subjectPath); err == nil {
		subjectID = strings.TrimSpace(string(b))
	}
	if v, ok := os.LookupEnv(subjectVar); ok {
		subjectID = v
	}

	names, paths := findPrograms(programsDir, runtime.GOOS)
	load := func(path string) ([]entry, error) {
		return readSession(path, paths, programsDir, exec.LookPath, runtime.GOOS)
	}

	session := flag.Arg(0)
	if *printOnly {
		if session == "" {
			fatalf("-print needs a SESSION_FILE")
		}
		path := resolveSession(session, cwd, root)
		entries, err := load(path)
		if err != nil {
			fatalf("%v", err)
		}
		if id, ok := subjectFromName(path); ok {
			subjectID = id
		}
		if validSubject(subjectID) != nil {
			fatalf("no Subject ID: name the session file sub-<ID>_..., or set %s", subjectVar)
		}
		if err := printCommands(os.Stdout, entries, common, subjectID); err != nil {
			fatalf("%v", err)
		}
		return
	}

	m := newMenu(root, filepath.Join(root, logFile), subjectPath, common, subjectID)
	if session != "" {
		path := resolveSession(session, cwd, root)
		entries, err := load(path)
		if err != nil {
			fatalf("%v", err)
		}
		m.openSession(path, entries)
	} else if sessions := listSessions(filepath.Join(root, sessionsDir)); len(sessions) > 0 {
		m.showPicker(sessions, load)
	} else {
		if len(names) == 0 {
			fatalf("no session file, no %s/*.txt, and no program in %s", sessionsDir, programsDir)
		}
		var entries []entry
		for _, name := range names {
			entries = append(entries, entry{line: name, prog: name, path: paths[name]})
		}
		m.openSession("", entries)
	}
	if err := m.runMenu(); err != nil {
		fatalf("%v", err)
	}
}
