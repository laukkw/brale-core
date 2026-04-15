package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestConfirmActionAssumeYes(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetErr(&bytes.Buffer{})
	ok, err := confirmAction(cmd, "confirm", true)
	if err != nil {
		t.Fatalf("confirmAction: %v", err)
	}
	if !ok {
		t.Fatalf("expected true")
	}
}

func TestConfirmActionYesInput(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetErr(&bytes.Buffer{})
	ok, err := confirmAction(cmd, "confirm", false)
	if err != nil {
		t.Fatalf("confirmAction: %v", err)
	}
	if !ok {
		t.Fatalf("expected true")
	}
}

func TestConfirmActionDefaultNo(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("\n"))
	cmd.SetErr(&bytes.Buffer{})
	ok, err := confirmAction(cmd, "confirm", false)
	if err != nil {
		t.Fatalf("confirmAction: %v", err)
	}
	if ok {
		t.Fatalf("expected false")
	}
}
