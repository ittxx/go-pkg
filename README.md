# Go Skeleton

A collection of reusable Go packages with best practices for building microservices and applications.

## Installation

```bash
go get github.com/ittxx/go-pkg
```

## Packages

### Configuration (`pkg/config`)

Universal configuration system that works with any struct:

```go
type MyConfig struct {
    ServerHost string `yaml:"server_host" env:"SERVER_HOST" default:"localhost"`
    ServerPort int    `yaml:"server_port" env:"SERVER_PORT" default:"8080"`
    Debug      bool   `yaml:"debug" env:"DEBUG" default:"false"`
}

// Multiple sources with priority
configManager := config.New().
    AddSource(config.NewYAMLSource("config.yaml")).
    AddSource(config.NewEnvSource("APP__"))

cfg := &MyConfig{}
if err := configManager.Load(cfg); err != nil {
    log.Fatal(err)
}

// Simple YAML + ENV
err := config.LoadFromYAMLAndENV("config.yaml", cfg)
```

**Features:**
- Multiple sources: ENV > YAML > Defaults
- Nested struct support
- Field validation
- Default values via tags

### Logger (`pkg/logger`)

Structured logging based on Go 1.21+ slog:

```go
log := logger.New(logger.Config{
    Level:       "info",
    Format:      "json",
    ServiceName: "my-service",
})

// Structured logging
log.Info("User created",
    "user_id", 123,
    "email", "user@example.com",
)

// With context
log.WithContext(ctx).Info("Operation completed")

// Specialized methods
log.HTTPRequest(ctx, "GET", "/api/users", 200, duration)
log.DatabaseOperation(ctx, "SELECT", "users", duration, err)
log.Business(ctx, "create_user", "user_id", 123)
```

**Features:**
- Built on Go 1.21+ slog (no external dependencies)
- Context-aware logging
- Specialized methods for HTTP, DB, and business operations
- JSON/text output formats

### Metrics (`pkg/metrics`)

Modular Prometheus metrics:

```go
// HTTP metrics only
metrics := metrics.NewHTTPMetrics()

// Full metrics (HTTP + Database)
metrics := metrics.NewFullMetrics()

// Enable database modules later
metrics.WithDatabaseMetrics()

// Record metrics
metrics.RecordHTTPRequest("GET", "/api/users", 200, 0.123)
metrics.UpdateDBConnections(5, "active")
metrics.RecordDBQuery("SELECT", "users", 0.045, nil)

// Middleware
handler := metrics.MetricsMiddleware(metrics)(httpHandler)

// Prometheus endpoint
http.Handle("/metrics", metrics.Handler())
```

**Features:**
- Modular design (database metrics optional)
- Rich labels and histograms
- HTTP middleware included
- Safe calls to disabled modules

### Database (`pkg/database`)

Database abstractions for PostgreSQL:

```go
// Using pgx/v5
db, err := database.NewPostgreSQL(ctx, &cfg.Database, log, metrics)
if err != nil {
    return err
}
defer db.Close()

// Health check
if err := db.Health(ctx); err != nil {
    log.Error("Database unavailable", err)
}

// Get statistics
stats := db.GetStats()
```

**Features:**
- PostgreSQL support via pgx/v5
- Connection pooling
- Health checks
- Statistics monitoring

### Migrations (`pkg/migrations`)

Database migration system:

```go
// Define migration
type MyMigration struct{}

func (m *MyMigration) Version() string { return "001" }
func (m *MyMigration) Description() string { return "Create users table" }
func (m *MyMigration) Up(ctx context.Context, db *sql.DB) error {
    _, err := db.ExecContext(ctx, `CREATE TABLE users (...)`)
    return err
}
func (m *MyMigration) Down(ctx context.Context, db *sql.DB) error {
    _, err := db.ExecContext(ctx, `DROP TABLE users`)
    return err
}

// Run migrations
migrator := migrations.NewMigrator(db, log)
err := migrator.Run([]migrations.Migration{&MyMigration{}})
```

## Quick Start

```go
package main

import (
    "github.com/your-org/go-skeleton/pkg/config"
    "github.com/your-org/go-skeleton/pkg/logger"
    "github.com/your-org/go-skeleton/pkg/metrics"
    "net/http"
)

type Config struct {
    Port int `yaml:"port" env:"PORT" default:"8080"`
}

func main() {
    // Load configuration
    var cfg Config
    if err := config.LoadFromYAMLAndENV("config.yaml", &cfg); err != nil {
        panic(err)
    }

    // Setup logger
    log := logger.New(logger.Config{
        Level:       "info",
        Format:      "json",
        ServiceName: "my-service",
    })

    // Setup metrics
    m := metrics.NewHTTPMetrics()
    m.Init()

    // HTTP server with metrics
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Write([]byte("Hello, World!"))
    })
    
    server := &http.Server{
        Addr:    fmt.Sprintf(":%d", cfg.Port),
        Handler: metrics.MetricsMiddleware(m)(handler),
    }
    
    log.Info("Server started", "port", cfg.Port)
    if err := server.ListenAndServe(); err != nil {
        log.Fatal("Server error", err)
    }
}
```

## Configuration

### YAML Example

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: "15s"
  write_timeout: "15s"

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "postgres"
  database: "service_db"
  ssl_mode: "disable"
  max_conns: 25

logging:
  level: "info"
  format: "json"
  service_name: "my-service"

metrics:
  enabled: true
  port: 9090
```

### Environment Variables

```bash
APP__SERVER__HOST=0.0.0.0
APP__SERVER__PORT=8080
APP__DATABASE__HOST=localhost
APP__LOGGING__LEVEL=debug
```

## Architecture Principles

1. **Minimal dependencies** - each package has minimal external dependencies
2. **Standalone usage** - packages work independently
3. **Type safety** - strong typing and validation
4. **Performance** - efficient implementations
5. **Production ready** - battle-tested in production

## License

MIT License
