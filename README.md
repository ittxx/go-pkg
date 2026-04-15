# go-pkg

go-pkg is a small collection of reusable Go packages and lightweight examples intended as a starter library for service projects. It focuses on pragmatic defaults, small surface area and clear, testable utilities.

Primary packages
- config — YAML/ENV configuration loader with support for defaults and durations.
- logger — Thin wrapper around Go's slog with helper methods for structured logging.
- database — Utilities for SQL database handling, connection monitoring and migrations.
- metrics — Modular metrics helpers for HTTP and database instrumentation.
- migrations — Helpers to load SQL migrations from fs.FS and a safe migration runner.

Quick start
1. Run tests:
   cd go-pkg && go test ./...
2. Example (no DB required):
   go run ./examples/basic
3. Advanced example (Postgres required):
   go run ./examples/advanced
   Configure connection in the example's config or via environment variables.

Migrations
- A single, canonical migrator implementation lives in pkg/database/migrator.go.
  It supports:
  - Programmatic migrations (Migration interface).
  - Recording applied versions in a `schema_migrations` table.
  - Rollback of the last applied migration.
  - Loading SQL migrations from an embedded fs.FS via `LoadMigrationsFromFS`.
  - A small helper `RunDirectoryMigrations` to apply `*.up.sql` files from a directory.
- pkg/migrations provides thin aliases/wrappers that delegate to pkg/database to keep backward compatibility.

Logging
- pkg/logger provides a simple structured logger wrapper with helpers:
  - WithComponent, WithError, WithRequestID, WithTraceID, WithContext.
  - Convenience functions for common patterns: Request, Business, HTTPRequest, DatabaseOperation, Performance.
- Default format: JSON. Default level: info.
- Use `logger.InitGlobal` to initialize the global logger, or create local instances with `logger.New`.

Testing
- Unit tests use sqlmock where appropriate for database-related packages.
- To run all tests with coverage:
  cd go-pkg && go test ./... -cover

Contributing
- Small, focused changes are preferred.
- Keep public package surfaces minimal and documented.
- Add unit tests for new behavior and run `go vet` / `golangci-lint` if available.

Notes and recommendations
- The repository aims to be compact and clear for publishing as a reusable utility collection. If you want stricter guarantees (migrations, transactional guarantees), prefer the programmatic migrator in pkg/database over simple directory-based runners.
- Consider adding CI (GitHub Actions) to run tests and check coverage automatically.

License
- Default repository license: MIT (please add or update LICENSE file as needed).

Contact
- If you want changes or additional examples (e.g., CI workflow, expanded metrics examples), open an issue or request the change in the repository.
