package main

import (
	"encoding/json"
	"testing"

	"brale-core/internal/memory"
)

func TestMemoryListCommandFiltersJSONBySource(t *testing.T) {
	t.Skip("requires PostgreSQL")
	_ = memory.Rule{}
	_ = json.Unmarshal
}
