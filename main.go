// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

// session-menu runs the programs of a session, one after the other, from a
// terminal menu:
//
//	session-menu [-dir DIR] [SESSION_FILE]
//
// A session file lists one command per line: a program, then its options. A
// program is looked up in DIR/bin, then in the PATH. Without a session file,
// the menu lists every program of DIR/bin.
//
// The words of DIR/command_line_args.txt, if it exists, are inserted right
// after the program of every line, except the lines that start with '!',
// which run as written. "{subject}" is replaced by the Subject ID, in that
// file and in the session lines.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	quit        = "Quit"
	binDir      = "bin"
	argsFile    = "command_line_args.txt"
	subjectFile = ".last_subject.txt"
	// subjectVar is set to the Subject ID in the environment of the programs.
	subjectVar = "SUBJECT"
	// placeholder is replaced by the Subject ID in the command lines.
	placeholder = "{subject}"
)

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// findPrograms returns the sorted names of the programs in dir, and the path
// of the executable to run for each of them. goos selects the layout:
// windows: <name>.exe; others: executable files, and on darwin also
// <name>.app bundles.
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
			if !strings.HasSuffix(strings.ToLower(name), ".exe") || !isFile(path) {
				continue
			}
			name = name[:len(name)-len(".exe")]
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
	line string // as listed in the menu
	prog string
	args []string
	path string // executable to run
	// the line started with '!': run as written, without the common arguments
	raw bool
}

// readSession returns the programs listed in a session file.
// One program per line, optionally followed by options passed to it.
// Blank lines and lines starting with '#' are ignored; a line starting with
// '!' gets no common arguments. A program is looked up in available (the
// programs of bin/), then with lookPath (exec.LookPath: in the PATH).
func readSession(path string, available map[string]string, programsDir string,
	lookPath func(string) (string, error)) ([]entry, error) {
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
		e := entry{line: line}
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
			return nil, fmt.Errorf("%s:%d: '%s' not found in %s nor in the PATH", path, lineno, e.prog, programsDir)
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

// run runs cmd in the terminal and returns true if it succeeded. On failure,
// it keeps the output on screen until the user has read it.
func run(cmd []string) bool {
	fmt.Printf("\n%s\n", shellJoin(cmd))

	// Ctrl-C stops the program, not the menu
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	c := exec.Command(cmd[0], cmd[1:]...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	err := c.Run()
	if err == nil {
		return true
	}
	name := filepath.Base(cmd[0])
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		fmt.Printf("\n*** %s exited with code %d ***\n", name, exitErr.ExitCode())
	case errors.As(err, &exitErr): // killed by a signal
		fmt.Printf("\n*** %s: %v ***\n", name, err)
	default:
		fmt.Printf("Command not found: %s (%v)\n", cmd[0], err)
	}
	fmt.Print("\nPress Enter to return to menu...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	return false
}

func main() {
	dir := flag.String("dir", "", "project folder, with bin/, "+argsFile+" and "+subjectFile+
		" (default: the current directory)")
	showVersion := flag.Bool("version", false, "print the version of this program and exit")
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [-dir DIR] [-version] [SESSION_FILE]\n\n"+
			"SESSION_FILE: file listing the commands to run, one per line "+
			"(default: all the programs of DIR/bin)\n\n",
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

	// The Subject ID field starts with: the ID in the name of the session
	// file, else $SUBJECT, else the last subject.
	subjectID := ""
	if b, err := os.ReadFile(subjectPath); err == nil {
		subjectID = strings.TrimSpace(string(b))
	}
	if v, ok := os.LookupEnv(subjectVar); ok {
		subjectID = v
	}

	names, paths := findPrograms(programsDir, runtime.GOOS)
	var programs []entry
	title := "Session"
	if session := flag.Arg(0); session != "" {
		programs, err = readSession(session, paths, programsDir, exec.LookPath)
		if err != nil {
			fatalf("%v", err)
		}
		title = strings.TrimSuffix(filepath.Base(session), filepath.Ext(session))
		if id, ok := subjectFromName(session); ok {
			subjectID = id
		}
	} else {
		if len(names) == 0 {
			fatalf("no session file, and no program in %s", programsDir)
		}
		for _, name := range names {
			programs = append(programs, entry{line: name, prog: name, path: paths[name]})
		}
	}

	header := func(id string) string {
		args := "(none: no " + argsFile + ")"
		if len(common) > 0 {
			args = shellJoin(common)
		}
		return fmt.Sprintf("Time=%s | %s=%s | Common args: %s", time.Now().Format("15:04:05"), subjectVar, id, args)
	}

	app := tview.NewApplication()
	pages := tview.NewPages()

	// 1. Info Header
	info := tview.NewTextView().SetText(header(subjectID)).SetTextColor(tcell.ColorGreen)

	// 2. Direct Subject Input Widget
	subject := tview.NewInputField().SetLabel("Subject ID: ").SetText(subjectID)

	// 3. The Action Menu: one entry per program
	menu := tview.NewList().ShowSecondaryText(false).SetWrapAround(false)
	menu.SetBorder(true).SetTitle(" Select Action (TAB to switch between Subject ID and Menu) ")
	for i, p := range programs {
		menu.AddItem(tview.Escape(fmt.Sprintf("%d. %s", i+1, p.line)), "", 0, nil)
	}
	menu.AddItem(quit, "", 0, nil)

	showError := func(msg string) {
		modal := tview.NewModal().SetText(msg).AddButtons([]string{"OK"}).
			SetDoneFunc(func(int, string) {
				pages.RemovePage("error")
				app.SetFocus(subject)
			})
		pages.AddPage("error", modal, false, true)
		app.SetFocus(modal)
	}

	menu.SetSelectedFunc(func(index int, _, _ string, _ rune) {
		if index == menu.GetItemCount()-1 {
			app.Stop()
			return
		}

		// Grab the Subject ID currently typed in the field above
		id := strings.TrimSpace(subject.GetText())
		if err := validSubject(id); err != nil {
			showError(err.Error())
			return
		}
		os.Setenv(subjectVar, id)
		info.SetText(header(id))
		// Save to File (Persistence); don't stop if it can't be written
		os.WriteFile(subjectPath, []byte(id), 0o644)

		cmd := command(programs[index], common, id)

		// Leave the full screen so that the program's output goes to a
		// normal terminal
		ok := false
		app.Suspend(func() { ok = run(cmd) })
		info.SetText(header(id))

		// Move the cursor to the next entry (stop before Quit), only on success
		if ok && index < menu.GetItemCount()-2 {
			menu.SetCurrentItem(index + 1)
		}
	})

	// TAB switches between the Subject ID field and the menu (Enter in the
	// field too)
	subject.SetDoneFunc(func(tcell.Key) { app.SetFocus(menu) })
	app.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		if pages.HasPage("error") || (ev.Key() != tcell.KeyTab && ev.Key() != tcell.KeyBacktab) {
			return ev
		}
		if subject.HasFocus() {
			app.SetFocus(menu)
		} else {
			app.SetFocus(subject)
		}
		return nil
	})

	form := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(info, 1, 0, false).
		AddItem(nil, 1, 0, false). // Spacer
		AddItem(subject, 1, 0, true).
		AddItem(nil, 1, 0, false). // Spacer
		AddItem(menu, 0, 1, false)
	form.SetBorder(true).SetTitle(" " + title + " ")
	pages.AddPage("main", form, true, true)

	if err := app.SetRoot(pages, true).SetFocus(subject).Run(); err != nil {
		fatalf("%v", err)
	}
}
