// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

// The log of the runs, session-menu-log.csv, and the marks read from it.

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

// result is how a run went: when the program started and ended, and its exit
// code.
type result struct {
	start, end time.Time
	exitCode   int // -1: killed by a signal, or could not be started
}

// exitCode returns the exit code of a program whose run ended with err (from
// exec.Cmd.Run): 0 on success, -1 if it was killed by a signal or could not
// be started.
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() // -1 for a signal
	}
	return -1
}

// logHeader are the columns of the log.
var logHeader = []string{"start", "end", "duration_s", "exit_code", "subject", "session", "line", "command"}

// appendLog appends a run to the log at path, a CSV file created with its
// header if it does not exist yet. session is the session file as sessionKey
// names it ("" without one), lineno its line (0 without one), and cmd the
// command line as it was run.
func appendLog(path string, r result, subject, session string, lineno int, cmd []string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w := csv.NewWriter(f)
	if fi.Size() == 0 {
		w.Write(logHeader)
	}
	line := ""
	if lineno > 0 {
		line = strconv.Itoa(lineno)
	}
	w.Write([]string{
		r.start.Format(timeFormat), r.end.Format(timeFormat),
		strconv.FormatFloat(r.end.Sub(r.start).Seconds(), 'f', 3, 64),
		strconv.Itoa(r.exitCode), subject, session, line, shellJoin(cmd),
	})
	w.Flush()
	if err := w.Error(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// loadMarks returns the exit code of the last run of each line of the session
// file session (as sessionKey names it) for subject, read from the log at
// path. A missing log gives none.
func loadMarks(path, subject, session string) (map[int]int, error) {
	marks := map[int]int{}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return marks, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %v", path, err)
	}
	if len(rows) == 0 {
		return marks, nil
	}
	col := map[string]int{}
	for i, c := range rows[0] {
		col[c] = i
	}
	for _, c := range []string{"exit_code", "subject", "session", "line"} {
		if _, ok := col[c]; !ok {
			return nil, fmt.Errorf("%s: no column %q", path, c)
		}
	}
	field := func(row []string, c string) string {
		if i := col[c]; i < len(row) {
			return row[i]
		}
		return ""
	}
	for _, row := range rows[1:] {
		if field(row, "subject") != subject || field(row, "session") != session {
			continue
		}
		line, err1 := strconv.Atoi(field(row, "line"))
		code, err2 := strconv.Atoi(field(row, "exit_code"))
		if err1 == nil && err2 == nil {
			marks[line] = code // the last run wins
		}
	}
	return marks, nil
}
