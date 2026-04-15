package main

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
)

func printJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func printTable(w io.Writer, headers []string, rows [][]string) error {
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	for i, h := range headers {
		if i > 0 {
			if _, err := fmt.Fprint(tw, "\t"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprint(tw, h); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(tw); err != nil {
		return err
	}
	for _, row := range rows {
		for i, col := range row {
			if i > 0 {
				if _, err := fmt.Fprint(tw, "\t"); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprint(tw, col); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(tw); err != nil {
			return err
		}
	}
	return tw.Flush()
}

func printBlock(w io.Writer, text string) error {
	if text == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, text)
	return err
}
