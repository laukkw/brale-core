package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewRootCmdRegistersGlobalFlags(t *testing.T) {
	cmd := newRootCmd()
	if cmd == nil {
		t.Fatalf("newRootCmd returned nil")
	}
	if cmd.PersistentFlags().Lookup("endpoint") == nil {
		t.Fatalf("endpoint flag not registered")
	}
	if cmd.PersistentFlags().Lookup("json") == nil {
		t.Fatalf("json flag not registered")
	}
}

func TestRunCLIHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runCLI([]string{"--help"}, &stdout, &stderr); err != nil {
		t.Fatalf("runCLI: %v", err)
	}
	if !strings.Contains(stdout.String(), "bralectl") {
		t.Fatalf("stdout=%s", stdout.String())
	}
}
