package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func confirmAction(cmd *cobra.Command, prompt string, assumeYes bool) (bool, error) {
	if assumeYes {
		return true, nil
	}
	if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N]: ", prompt); err != nil {
		return false, err
	}
	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", nil
}
