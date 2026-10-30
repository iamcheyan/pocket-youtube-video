# tests/

Black-box integration coverage for the assembled `videod` stack (HTTP surface,
SQLite persistence, stub yt-dlp queue, credential file permissions, account
isolation). Go's `internal/` rule requires these tests to live inside the backend
module, so the executable suite is at `backend/tests/integration_test.go`; this
directory documents the contract.

Run it with `make test-integration` (or it runs as part of `make test`).
