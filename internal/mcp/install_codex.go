package mcp

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/creachadair/tomledit"
	"github.com/creachadair/tomledit/parser"
)

func installCodexConfig(prepared preparedInstall) error {
	doc, err := loadCodexInstallDocument(prepared.configPath)
	if err != nil {
		return err
	}
	if err := upsertCodexServer(doc, prepared); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := tomledit.Format(&buf, doc); err != nil {
		return fmt.Errorf("format codex config: %w", err)
	}
	if err := writeAtomic(prepared.configPath, buf.Bytes()); err != nil {
		return fmt.Errorf("write codex config: %w", err)
	}
	return nil
}

func loadCodexInstallDocument(path string) (*tomledit.Document, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			raw = nil
		} else {
			return nil, fmt.Errorf("read codex config: %w", err)
		}
	}
	doc, err := tomledit.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse codex config: %w", err)
	}
	return doc, nil
}

func upsertCodexServer(doc *tomledit.Document, prepared preparedInstall) error {
	if doc == nil {
		return nil
	}
	key := parser.Key{"mcp_servers", prepared.name}
	filtered := doc.Sections[:0]
	for _, section := range doc.Sections {
		if section == nil || !section.TableName().Equals(key) {
			filtered = append(filtered, section)
		}
	}
	doc.Sections = filtered
	items, err := buildCodexServerItems(prepared)
	if err != nil {
		return err
	}
	doc.Sections = append(doc.Sections, &tomledit.Section{
		Heading: &parser.Heading{Name: key},
		Items:   items,
	})
	return nil
}

func buildCodexServerItems(prepared preparedInstall) ([]parser.Item, error) {
	switch prepared.mode {
	case "stdio":
		return []parser.Item{
			&parser.KeyValue{
				Name:  parser.Key{"command"},
				Value: parser.MustValue(strconv.Quote(prepared.command)),
			},
			&parser.KeyValue{
				Name:  parser.Key{"args"},
				Value: parser.MustValue(renderTOMLStringArray(prepared.args)),
			},
		}, nil
	case "http":
		return []parser.Item{
			&parser.KeyValue{
				Name:  parser.Key{"url"},
				Value: parser.MustValue(strconv.Quote(prepared.httpURL)),
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported install mode %q", prepared.mode)
	}
}

func renderTOMLStringArray(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	buf := bytes.NewBufferString("[")
	for i, value := range values {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString(strconv.Quote(value))
	}
	buf.WriteString("]")
	return buf.String()
}
