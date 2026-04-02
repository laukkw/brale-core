# Refactor Follow-Ups

## Deferred Cleanup

1. Refactor exchange transport readability hotspots.
   - `internal/market/binance/futures.go`
   - Split by endpoint or concern without changing wire behavior.

## Verification Debt

1. Re-run full package validation before closing the branch.
   - `go test ./...`
   - `go test -race ./...`

2. Add more characterization tests for refactored boundaries.
   - `internal/runtime/webhook_sync_service.go`
   - `internal/transport/runtimeapi/usecase_dashboard_flow.go`
