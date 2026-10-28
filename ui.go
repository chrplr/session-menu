// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

// The terminal menu: the list of sessions, and the programs of a session.

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// guard is how long the menu ignores the keyboard after a program returns.
// Otherwise an Enter pressed twice to leave a program starts the next one: a
// second Enter 0.05-0.3 s after the first did, in a test under tmux.
const guard = 500 * time.Millisecond

// run runs cmd in the folder dir, in the terminal. On failure, it keeps the
// output on screen until the user has read it; the end of the run is taken
// before that.
func run(cmd []string, dir string) result {
	fmt.Printf("\n%s\n", shellJoin(cmd))

	// Ctrl-C stops the program, not the menu
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	c := exec.Command(cmd[0], cmd[1:]...)
	c.Dir = dir
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	start := time.Now()
	err := c.Run()
	res := result{start: start, end: time.Now(), exitCode: exitCode(err)}
	if err == nil {
		return res
	}
	name := filepath.Base(cmd[0])
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr) && exitErr.ExitCode() >= 0:
		fmt.Printf("\n*** %s exited with code %d ***\n", name, exitErr.ExitCode())
	case errors.As(err, &exitErr): // killed by a signal
		fmt.Printf("\n*** %s: %v ***\n", name, err)
	default:
		fmt.Printf("Could not start %s (%v)\n", cmd[0], err)
	}
	fmt.Print("\nPress Enter to return to menu...")
	bufio.NewReader(os.Stdin).ReadString('\n')
	return res
}

// markText is the text of entry i of a session in the list: the result of
// its last run if ran (a check mark, or a cross with the exit code), then
// its number and its line.
func markText(i int, e entry, code int, ran bool) string {
	mark := ""
	switch {
	case !ran:
	case code == 0:
		mark = "✓"
	case code > 0:
		mark = fmt.Sprintf("✗ exit %d", code)
	default: // killed by a signal, or not started
		mark = "✗ failed"
	}
	return tview.Escape(fmt.Sprintf("%-10s %d. %s", mark, i+1, e.line))
}

// menu is the state of the terminal menu.
type menu struct {
	app   *tview.Application
	pages *tview.Pages

	root        string   // project folder, where the programs run
	logPath     string   // the log of the runs
	subjectPath string   // the last Subject ID
	common      []string // the common arguments
	header      string   // first line of every page

	// The session files of the picker (none: no picker), and how to read one.
	sessions []string
	load     func(path string) ([]entry, error)
	picker   *tview.List

	// The session shown: its file as the log names it ("" for the programs
	// of bin/), its entries, and the exit code of the last run of each entry
	// (by index), for the Subject ID marksFor.
	session  string
	entries  []entry
	marks    map[int]int
	marksFor string
	field    *tview.InputField
	list     *tview.List

	subjectID string          // the Subject ID last typed
	resumed   time.Time       // when the last program returned
	focus     tview.Primitive // what has the focus when the menu starts
}

func newMenu(root, logPath, subjectPath string, common []string, subjectID string) *menu {
	args := "(none: no " + argsFile + ")"
	if len(common) > 0 {
		args = shellJoin(common)
	}
	m := &menu{
		app:         tview.NewApplication(),
		pages:       tview.NewPages(),
		root:        root,
		logPath:     logPath,
		subjectPath: subjectPath,
		common:      common,
		header:      fmt.Sprintf("Project: %s | Common args: %s", homeTilde(root), args),
		subjectID:   subjectID,
	}
	m.app.SetInputCapture(m.keys)
	return m
}

// homeTilde writes path with ~ for the home folder, to keep it short.
func homeTilde(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.Join("~", rel)
	}
	return path
}

// page frames the items of a page: the header at the top, the help line at
// the bottom.
func (m *menu) page(title, help string, items ...tview.Primitive) *tview.Flex {
	f := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tview.NewTextView().SetText(m.header).SetTextColor(tcell.ColorGreen), 1, 0, false).
		AddItem(nil, 1, 0, false)
	for _, it := range items {
		if _, isList := it.(*tview.List); isList {
			f.AddItem(it, 0, 1, false)
		} else {
			f.AddItem(it, 1, 0, false)
		}
	}
	f.AddItem(tview.NewTextView().SetText(help).SetTextColor(tcell.ColorGray), 1, 0, false)
	f.SetBorder(true).SetTitle(" " + title + " ")
	return f
}

// showPicker adds the list of the session files, and shows it.
func (m *menu) showPicker(sessions []string, load func(string) ([]entry, error)) {
	m.sessions, m.load = sessions, load
	m.picker = tview.NewList().ShowSecondaryText(false).SetWrapAround(false)
	for _, path := range sessions {
		m.picker.AddItem(tview.Escape(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))), "", 0, nil)
	}
	m.picker.AddItem(quit, "", 0, nil)
	m.picker.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i == len(m.sessions) {
			m.app.Stop()
			return
		}
		entries, err := m.load(m.sessions[i])
		if err != nil {
			m.showError(err.Error(), m.picker)
			return
		}
		m.openSession(m.sessions[i], entries)
	})
	m.pages.AddPage("picker", m.page("Sessions", "Enter: open   q: quit", m.picker), true, true)
	m.setFocus(m.picker)
}

// openSession shows the entries of the session file at path ("" for the
// programs of bin/), with the marks of the Subject ID, and the cursor on the
// first entry not done.
func (m *menu) openSession(path string, entries []entry) {
	m.session, m.entries = sessionKey(path, m.root), entries
	title := "Programs of bin/"
	if path != "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if id, ok := subjectFromName(path); ok {
			m.subjectID = id
		}
	}

	m.list = tview.NewList().ShowSecondaryText(false).SetWrapAround(false)
	for i, e := range entries {
		m.list.AddItem(markText(i, e, 0, false), "", 0, nil)
	}
	m.list.AddItem(fmt.Sprintf("%-10s %s", "", quit), "", 0, nil) // under the lines
	m.list.SetSelectedFunc(func(i int, _, _ string, _ rune) {
		if i == len(m.entries) {
			m.app.Stop()
			return
		}
		m.runEntry(i)
	})

	m.field = tview.NewInputField().SetLabel("Subject ID: ").SetText(m.subjectID)
	m.field.SetChangedFunc(func(text string) {
		m.subjectID = strings.TrimSpace(text)
		m.loadMarks()
	})
	m.field.SetDoneFunc(func(tcell.Key) { m.app.SetFocus(m.list) })

	m.marksFor = "\x00" // no Subject ID: loadMarks reads the log
	m.loadMarks()
	next := len(entries) // Quit, if all are done
	for i := range entries {
		if code, ran := m.marks[i]; !ran || code != 0 {
			next = i
			break
		}
	}
	m.list.SetCurrentItem(next)

	help := "Enter: run   Tab: Subject ID   q: quit"
	if len(m.sessions) > 0 {
		help = "Enter: run   Tab: Subject ID   Esc: sessions   q: quit"
	}
	m.pages.AddAndSwitchToPage("session", m.page(title, help, m.field, nil, m.list), true)
	m.setFocus(m.field)
}

// loadMarks reads the marks of the Subject ID from the log, if they are not
// those shown, and shows them. The programs of bin/, without a session file,
// have no line numbers: their marks are only those of this menu.
func (m *menu) loadMarks() {
	if m.subjectID == m.marksFor {
		return
	}
	m.marks, m.marksFor = map[int]int{}, m.subjectID
	if m.session != "" && validSubject(m.subjectID) == nil {
		byLine, err := loadMarks(m.logPath, m.subjectID, m.session)
		if err != nil {
			m.showError("Could not read the log: "+err.Error(), m.list)
		}
		for i, e := range m.entries {
			if code, ok := byLine[e.lineno]; ok {
				m.marks[i] = code
			}
		}
	}
	for i, e := range m.entries {
		code, ran := m.marks[i]
		m.list.SetItemText(i, markText(i, e, code, ran), "")
	}
}

// runEntry runs entry i, logs it, marks it, and moves the cursor to the next
// entry if it succeeded.
func (m *menu) runEntry(i int) {
	id := strings.TrimSpace(m.field.GetText())
	if err := validSubject(id); err != nil {
		m.showError(err.Error(), m.field)
		return
	}
	m.subjectID = id
	m.loadMarks()
	os.Setenv(subjectVar, id)
	// Save to File (Persistence); don't stop if it can't be written
	os.WriteFile(m.subjectPath, []byte(id), 0o644)

	e := m.entries[i]
	cmd := command(e, m.common, id)
	// Leave the full screen so that the program's output goes to a normal
	// terminal
	var res result
	m.app.Suspend(func() { res = run(cmd, m.root) })
	m.resumed = time.Now()

	m.marks[i] = res.exitCode
	m.list.SetItemText(i, markText(i, e, res.exitCode, true), "")
	if res.exitCode == 0 && i < len(m.entries)-1 {
		m.list.SetCurrentItem(i + 1)
	}
	if err := appendLog(m.logPath, res, id, m.session, e.lineno, cmd); err != nil {
		m.showError(fmt.Sprintf("Could not write the log: %v", err), m.list)
	}
}

// showError shows msg in a dialog, then gives the focus to back.
func (m *menu) showError(msg string, back tview.Primitive) {
	modal := tview.NewModal().SetText(msg).AddButtons([]string{"OK"}).
		SetDoneFunc(func(int, string) {
			m.pages.RemovePage("error")
			m.app.SetFocus(back)
		})
	m.pages.AddPage("error", modal, false, true)
	m.app.SetFocus(modal)
}

// keys handles the keys of every page: TAB between the Subject ID and the
// list, q to quit from a list, Esc back to the sessions. All keys are
// ignored for a moment after a program returns (see guard).
func (m *menu) keys(ev *tcell.EventKey) *tcell.EventKey {
	if time.Since(m.resumed) < guard {
		return nil
	}
	if m.pages.HasPage("error") {
		return ev
	}
	front, _ := m.pages.GetFrontPage()
	inList := (front == "picker" && m.picker.HasFocus()) || (front == "session" && m.list.HasFocus())
	switch {
	case ev.Key() == tcell.KeyRune && ev.Rune() == 'q' && inList:
		m.app.Stop()
	case front == "session" && (ev.Key() == tcell.KeyTab || ev.Key() == tcell.KeyBacktab):
		if m.field.HasFocus() {
			m.app.SetFocus(m.list)
		} else {
			m.app.SetFocus(m.field)
		}
	case front == "session" && ev.Key() == tcell.KeyEscape && len(m.sessions) > 0:
		m.pages.RemovePage("session")
		m.pages.SwitchToPage("picker")
		m.app.SetFocus(m.picker)
	default:
		return ev
	}
	return nil
}

// setFocus gives the focus to p, now or, before the menu runs, when it
// starts (SetRoot would take it back).
func (m *menu) setFocus(p tview.Primitive) {
	m.focus = p
	m.app.SetFocus(p)
}

// runMenu runs the menu until Quit.
func (m *menu) runMenu() error {
	return m.app.SetRoot(m.pages, true).SetFocus(m.focus).Run()
}
