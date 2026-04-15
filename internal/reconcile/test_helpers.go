package reconcile

import (
	"testing"
)

func newTestDB(t *testing.T) {
	t.Helper()
	t.Skip("requires PostgreSQL")
}
