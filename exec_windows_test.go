// Copyright (2026) Christophe Pallier <christophe@pallier.org>
// Licensed under the Apache License, Version 2.0 (see LICENSE).

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestRunBat runs a .bat of bin/ as the menu does, and checks the arguments
// it gets, one with a blank included.
func TestRunBat(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	// one argument per line, without its quotes
	write(t, filepath.Join(bin, "task.bat"),
		"@echo off\r\n(for %%a in (%*) do echo %%~a) > \"%~dp0out.txt\"\r\n", 0o644)
	_, paths := findPrograms(bin, "windows")
	if paths["task"] == "" {
		t.Fatalf("findPrograms: no task in %v", paths)
	}
	cmd := command(entry{path: paths["task"], args: []string{"-title", "a b"}}, []string{"-s", "{subject}"}, "007")
	c := exec.Command(cmd[0], cmd[1:]...)
	c.Dir = dir
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v\n%s", cmd, err, out)
	}
	b, err := os.ReadFile(filepath.Join(bin, "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSpace(strings.ReplaceAll(string(b), "\r\n", "\n")), "\n")
	if want := []string{"-s", "007", "-title", "a b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("task.bat got %q, want %q", got, want)
	}
}
