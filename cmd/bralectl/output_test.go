package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintTable(t *testing.T) {
	var buf bytes.Buffer
	err := printTable(&buf, []string{"A", "B"}, [][]string{{"x", "y"}})
	if err != nil {
		t.Fatalf("printTable: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "A") || !strings.Contains(out, "x") {
		t.Fatalf("output=%q", out)
	}
}

func TestPrintBlockIgnoresEmptyText(t *testing.T) {
	var buf bytes.Buffer
	if err := printBlock(&buf, ""); err != nil {
		t.Fatalf("printBlock: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("buf=%q", buf.String())
	}
}

func TestCompactError(t *testing.T) {
	err := compactError(assertErr("line1\nline2"))
	if err != "line1 | line2" {
		t.Fatalf("compactError=%q", err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

func assertErr(msg string) error { return testError(msg) }
