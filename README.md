# register-api

Foundation for the National Population Register REST API. This repository
currently contains only the modular API skeleton: configuration, logging,
middleware, a health module, consistent JSON errors and graceful shutdown.
No business functionality, database or authentication is implemented yet.

## Stack

- Go standard library only: `net/http` with `http.ServeMux`, `log/slog`, `testing`.
- JSON request/response bodies.
- Environment-based configuration.
- No ORM, no database, no Docker, no external dependencies.

## Project layout

```text
.
├── cmd/api/main.go              # Entry point: config → logger → app → run
├── internal/
│   ├── app/app.go               # Composition root: builds modules, injects the server
│   ├── config/config.go         # Env parsing, defaults, startup validation
│   ├── server/
│   │   ├── server.go            # http.Server lifecycle, timeouts, graceful shutdown
│   │   └── routes.go            # Module interface, mux assembly, middleware chain
│   ├── middleware/
│   │   ├── request_id.go        # X-Request-ID generation/propagation
│   │   ├── logging.go           # One structured slog line per request, with timing
│   │   └── recovery.go          # Panic → logged 500 with the standard error envelope
│   ├── platform/
│   │   ├── logger/logger.go     # slog logger construction (text dev / JSON prod)
│   │   └── httpx/httpx.go       # JSON + error helpers, request-ID context
│   └── modules/
│       └── health/              # Reference module
│           ├── handler.go       # HTTP translation only
│           ├── service.go       # Business/health state, no HTTP knowledge
│           └── routes.go        # Registers GET /health on the mux
├── .env.example
├── .gitignore
├── go.mod
└── README.md
```

### Package responsibilities

| Package | Responsibility |
|---|---|
| `cmd/api` | Process entry point. Loads config, builds the logger, runs the app, exits non-zero on fatal startup errors. |
| `internal/app` | The only place that knows which modules exist. Constructs each module and passes it to the server. |
| `internal/config` | Reads environment variables, applies defaults, validates values before the server starts. |
| `internal/server` | Owns `http.Server`: timeouts, mux assembly, middleware chain, graceful shutdown. Defines the `Module` interface; never imports modules. |
| `internal/middleware` | Reusable `func(http.Handler) http.Handler` wrappers for request IDs, logging/timing and panic recovery. |
| `internal/platform/logger` | Builds `*slog.Logger` from `APP_ENV` and `LOG_LEVEL`. |
| `internal/platform/httpx` | JSON response and error envelope helpers; request-ID context accessors. |
| `internal/modules/health` | Example feature module: handler → service → routes. |

### How dependencies flow

```text
cmd/api  ──►  config.Load, logger.New
   │
   ▼
app.New(cfg, log)
   │  builds: health.NewService → health.NewHandler → health.NewRoutes
   ▼
server.New(cfg.Server, log, modules...)
   │  NewHandler: mux ← module.Register, wrapped by RequestID → Logging → Recovery
   ▼
Server.Run(ctx)  →  listens, then shuts down on SIGINT/SIGTERM
```

Dependencies point one way only. Handlers depend on their module's service and
`httpx`; modules do not import each other, and the server does not import
modules. Everything is constructor-injected from `app.New` — there are no
package-level variables or global state.

## Configuration

| Variable | Default | Notes |
|---|---|---|
| `APP_NAME` | `register-api` | Reported by `/health`. |
| `APP_ENV` | `development` | `production` selects JSON logs. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `SERVER_HOST` | `127.0.0.1` | Bind host. |
| `SERVER_PORT` | `8080` | Must be 1–65535. |
| `SERVER_READ_TIMEOUT` | `10s` | `time.ParseDuration` format. |
| `SERVER_WRITE_TIMEOUT` | `10s` | |
| `SERVER_IDLE_TIMEOUT` | `60s` | |
| `SERVER_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown budget. |

Invalid values (bad duration, unknown log level, out-of-range port, zero
timeouts) abort startup with a clear error. `.env.example` documents the full
set; there is intentionally no dotenv dependency, so export variables in your
shell or process manager.

## Build and run

```bash
go mod download
go build -o bin/register-api ./cmd/api
./bin/register-api
# or, without building:
go run ./cmd/api
```

Override settings per run:

```bash
SERVER_PORT=9090 LOG_LEVEL=debug go run ./cmd/api
```

Shut down cleanly with `Ctrl+C` (SIGINT) or SIGTERM.

## API

### Health

```bash
curl -i http://127.0.0.1:8080/health
```

```json
{"status":"ok","service":"register-api"}
```

### Error envelope

```bash
curl -i http://127.0.0.1:8080/does-not-exist
```

```json
{
  "error": {"code": "NOT_FOUND", "message": "Resource not found"},
  "request_id": "6f1c0c1f5b0e4c1f9d3d2c1b0a9f8e7d"
}
```

Every response carries an `X-Request-ID` header. A well-formed inbound
`X-Request-ID` is reused; otherwise a new one is generated. Unknown paths return
`404 NOT_FOUND`; a known path with an unsupported method returns
`405 METHOD_NOT_ALLOWED` with an `Allow` header. Internal failures always return
the generic `INTERNAL_ERROR` message — panics and their details are logged
server-side only. Error codes in use: `NOT_FOUND`, `METHOD_NOT_ALLOWED`,
`BAD_REQUEST`, `INTERNAL_ERROR`.

## Tests

```bash
go test ./...
go vet ./...
gofmt -l .
```

Coverage includes the health endpoint and method handling, JSON/error helpers,
request-ID middleware (generation, reuse, rejection of invalid IDs), panic
recovery (including that panic details do not leak), and configuration defaults,
overrides and validation.

## Adding a module

1. Create `internal/modules/<name>/` with `service.go`, `handler.go` and
   `routes.go`.
2. Give `routes.go` a `Register(mux *http.ServeMux)` method (it satisfies
   `server.Module` structurally).
3. Construct and inject it in `app.New`, passing the route set to
   `server.New`.

Planned modules — Identity, Electoral, Police, Tax, Grants, Audit, Event
synchronisation — are not implemented.
