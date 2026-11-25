# register-api

Foundation for the National Population Register REST API. This repository
currently contains only the modular API skeleton: configuration, logging,
middleware, a health module, consistent JSON errors and graceful shutdown.
No business functionality, database or authentication is implemented yet.

## Stack

- [Gin](https://github.com/gin-gonic/gin) (`gin-gonic/gin`) for routing and handlers.
- Go standard library for everything else: `net/http` server lifecycle, `log/slog`, `testing`.
- JSON request/response bodies.
- Environment-based configuration.
- No ORM, no database, no Docker.

## Project layout

```text
.
├── cmd/api/main.go              # Entry point: config → logger → app → run
├── internal/
│   ├── app/app.go               # Composition root: builds modules, injects the server
│   ├── config/config.go         # Env parsing, defaults, startup validation
│   ├── server/
│   │   ├── server.go            # http.Server lifecycle, timeouts, graceful shutdown
│   │   └── routes.go            # Module interface, Gin engine assembly, middleware
│   ├── middleware/
│   │   ├── request_id.go        # X-Request-ID generation/propagation
│   │   ├── logging.go           # One structured slog line per request, with timing
│   │   └── recovery.go          # Panic → logged 500 with the standard error envelope
│   ├── platform/
│   │   ├── httpx/httpx.go       # JSON + error helpers, request-ID context
│   │   ├── logger/logger.go     # slog logger construction (text dev / JSON prod)
│   │   └── router/router.go     # Route registry feeding the endpoint catalog
│   └── modules/
│       ├── health/              # Reference module
│       │   ├── handler.go       # HTTP translation only
│       │   ├── service.go       # Business/health state, no HTTP knowledge
│       │   └── routes.go        # Registers GET /health on the router
│       └── webui/               # Browser console (presentation only)
│           ├── index.html       # Embedded single-page UI
│           ├── handler.go       # Serves the page
│           └── routes.go        # Registers GET /
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
| `internal/server` | Owns `http.Server`: timeouts, Gin engine assembly, middleware chain, graceful shutdown. Defines the `Module` interface; never imports modules. |
| `internal/middleware` | Reusable Gin middleware (`gin.HandlerFunc`) for request IDs, logging/timing and panic recovery. |
| `internal/platform/logger` | Builds `*slog.Logger` from `APP_ENV` and `LOG_LEVEL`. |
| `internal/platform/httpx` | JSON response and error envelope helpers; request-ID context accessors. |
| `internal/platform/router` | Mounts module routes on the Gin engine while recording them, so the endpoint catalog always matches what is served. |
| `internal/modules/health` | Example feature module: handler → service → routes. |
| `internal/modules/webui` | Browser console served at `/`: health panel and request explorer. It calls the API over HTTP like any client, so it holds no business logic. |

### How dependencies flow

```text
cmd/api  ──►  config.Load, logger.New
   │
   ▼
app.New(cfg, log)
   │  builds: health.NewService → health.NewHandler → health.NewRoutes
   │          webui.NewHandler → webui.NewRoutes
   ▼
server.New(cfg.Server, log, modules...)
   │  NewHandler: gin.New() → router.New(engine) ← module.Register
   │  catalog at GET /api/endpoints; middleware RequestID → Logging → Recovery
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

### Web console

Open <http://127.0.0.1:8080/> in a browser. The page is embedded in the binary
and served by the API itself, so there is no separate frontend build or dev
server.

- **Service health** shows the `/health` result, response time and request ID,
  with a refresh button.
- **Available endpoints** is loaded from `GET /api/endpoints` and rendered
  automatically, so every mounted route appears with its method and summary. A
  **Use** button loads the endpoint into the request explorer.
- **Request explorer** sends GET/POST/PUT/PATCH/DELETE requests to any path on
  this API and shows the status, duration, request ID and response body.

The console is plain HTML/CSS/JavaScript and calls the API over HTTP with
same-origin `fetch`, so endpoints added by future modules work in the explorer
without changes to the page.

### Health

```bash
curl -i http://127.0.0.1:8080/health
```

```json
{"status":"ok","service":"register-api"}
```

### Endpoint catalog

```bash
curl http://127.0.0.1:8080/api/endpoints
```

```json
{
  "endpoints": [
    {"method": "GET", "path": "/", "summary": "Web console"},
    {"method": "GET", "path": "/api/endpoints", "summary": "List available endpoints"},
    {"method": "GET", "path": "/health", "summary": "Service health check"}
  ]
}
```

The catalog comes from the server's route registry, not a hand-written list, so
it cannot drift from the routes modules mount. The web console renders this same
payload. GET routes also answer HEAD requests, without appearing twice in the
catalog.

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

Coverage includes the health endpoint and method handling, the web console page
and its root-only route, the route registry and endpoint catalog, JSON/error
helpers, request-ID middleware (generation, reuse, rejection of invalid IDs),
panic recovery (including that panic details do not leak), routing fallbacks,
and configuration defaults, overrides and validation.

## Adding a module

1. Create `internal/modules/<name>/` with `service.go`, `handler.go` and
   `routes.go`.
2. Give `routes.go` a `Register(r *router.Router)` method (it satisfies
   `server.Module` structurally) and mount each endpoint with
   `r.Handle(method, path, summary, handler)`, where `handler` is a
   `gin.HandlerFunc`.
3. Construct and inject it in `app.New`, passing the route set to `server.New`.

The endpoint then appears automatically in `GET /api/endpoints` and in the web
console — no separate documentation list to update.

Planned modules — Identity, Electoral, Police, Tax, Grants, Audit, Event
synchronisation — are not implemented.
