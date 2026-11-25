# register-api

Foundation for the National Population Register REST API. The repository
contains the modular API skeleton and the initial MySQL data model: core
population records, security/audit, event delivery and backup metadata.

## Stack

- [Gin](https://github.com/gin-gonic/gin) (`gin-gonic/gin`) for routing and handlers.
- [GORM](https://gorm.io/) with MySQL for persistence and startup schema migration.
- Go standard library for `net/http` server lifecycle, `log/slog` and `testing`.
- JSON request/response bodies.
- Environment-based configuration.

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
│   ├── auth/auth.go             # OIDC verification and department/role authorization
│   ├── audit/writer.go          # Transactional, per-node audit hash chain
│   ├── crypto/transit.go        # Vault Transit encryption and HMAC client
│   ├── nid/nid.go               # National-ID format and check-digit validation
│   ├── db/
│   │   ├── models.go            # GORM models for the register schema
│   │   └── mysql.go             # MySQL connection pool and AutoMigrate
│   └── modules/
│       ├── identity/            # Protected identity and marriage operations
│       ├── registry/            # Health, grants, police and department-audit reads
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
| `internal/db` | Opens the MySQL connection pool and auto-migrates the register tables before serving HTTP. |
| `internal/server` | Owns `http.Server`: timeouts, Gin engine assembly, middleware chain, graceful shutdown. Defines the `Module` interface; never imports modules. |
| `internal/middleware` | Reusable Gin middleware (`gin.HandlerFunc`) for request IDs, logging/timing and panic recovery. |
| `internal/platform/logger` | Builds `*slog.Logger` from `APP_ENV` and `LOG_LEVEL`. |
| `internal/platform/httpx` | JSON response and error envelope helpers; request-ID context accessors. |
| `internal/platform/router` | Mounts module routes on the Gin engine while recording them, so the endpoint catalog always matches what is served. |
| `internal/auth` | Verifies OIDC bearer tokens and enforces active-official, department and role checks. |
| `internal/audit` | Appends audit records to a serialized, per-node SHA-256 hash chain; supports the operation transaction. |
| `internal/crypto` | Uses Vault Transit to encrypt national IDs and derive keyed lookup indexes. |
| `internal/nid` | Validates national-ID format and check digits before protected operations. |
| `internal/modules/health` | Example feature module: handler → service → routes. |
| `internal/modules/webui` | Browser console served at `/`: health panel and request explorer. It calls the API over HTTP like any client, so it holds no business logic. |
| `internal/modules/identity` | Protected person registration/lookups, address changes and marriage registration. |
| `internal/modules/registry` | Protected health, grants, police and department-audit reads. |

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
| `SERVER_HOST` | `127.0.0.1` | Bind host; non-loopback HTTP is rejected until HTTPS/mTLS is configured. |
| `SERVER_PORT` | `8080` | Must be 1–65535. |
| `SERVER_READ_TIMEOUT` | `10s` | `time.ParseDuration` format. |
| `SERVER_WRITE_TIMEOUT` | `10s` | |
| `SERVER_IDLE_TIMEOUT` | `60s` | |
| `SERVER_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown budget. |
| `DB_HOST` | `127.0.0.1` | MySQL server host. |
| `DB_PORT` | `3306` | MySQL server port. |
| `DB_USER` | — | Required MySQL user. |
| `DB_PASSWORD` | — | MySQL password; may be empty for local development. |
| `DB_NAME` | — | Required database/schema name. |
| `DB_CHARSET` | `utf8mb4` | MySQL connection character set. |
| `NODE_ID` | — | Required audit hash-chain identifier (max 20 characters). |
| `OIDC_ISSUER` | — | Required OIDC issuer URL used to verify bearer tokens. |
| `OIDC_AUDIENCE` | — | Required token audience. |
| `VAULT_ADDR` | — | Required Vault URL; remote Vault must use HTTPS. |
| `VAULT_TOKEN` | — | Required Vault Transit token; keep it secret. |
| `VAULT_TRANSIT_MOUNT` | `transit` | Transit mount name. |
| `VAULT_NID_ENCRYPTION_KEY` | — | Transit encryption key name for national IDs. |
| `VAULT_NID_HMAC_KEY` | — | Transit HMAC key name for national-ID indexes. |

Invalid values (bad duration, unknown log level, out-of-range port, zero
timeouts, non-loopback HTTP bind address, or missing database/auth/key-provider
settings) abort startup with a clear error.
At startup the app loads `.env` from the current working directory.
Copy `.env.example` to `.env` and set your values; variables already set in
the process environment take precedence. Missing or invalid `.env` files abort
startup with a clear error.

The configured database must already exist and the account needs permission to
create and alter tables. GORM `AutoMigrate` runs before the HTTP server starts
and creates/updates these tables: `persons`, `addresses`, `births`, `deaths`,
`marriages`, `biometric_templates`, `departments`, `officials`, `audit_log`,
`event_outbox`, `event_subscriptions`, `event_deliveries`, `backup_leases` and
`backup_runs`. This startup migration is convenient for development; production
deployments should review schema changes and consider running migrations with a
separate privileged account.

## Build and run

```bash
[ -f .env ] || cp .env.example .env
# Edit .env with your MySQL credentials before starting the API.
go mod download
go build -o bin/register-api ./cmd/api
./bin/register-api
# or, without building:
go run ./cmd/api
```

Override settings per run:

```bash
DB_USER=register_app DB_PASSWORD=change-me DB_NAME=register \
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
  this API and shows the status, duration, request ID and response body. Its
  optional Headers field accepts a JSON object, for example
  `{"Authorization":"Bearer <OIDC-ID-token>"}`. Headers are sent with each
  request until cleared or page reload; the UI does not persist them.

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

### Authenticated registry endpoints

Registry operations require an OIDC bearer token. The API verifies its issuer,
audience and lifetime, then loads an active official and enforces the route's
department and minimum role. Every authorized operation and denied request is
written to the node's audit hash chain; if that write fails, the request fails
closed.

| Method | Path | Minimum role | Operation |
|---|---|---|---|
| `POST` | `/v1/identity/persons` | identity supervisor | Register a person |
| `POST` | `/v1/identity/persons/lookup` | identity clerk | Look up a person by ID in the JSON body |
| `PATCH` | `/v1/identity/persons/{id}/address` | identity clerk | Close the current address and add a new one |
| `POST` | `/v1/identity/marriages` | identity supervisor | Record a marriage |
| `POST` | `/v1/health/persons/lookup` | health clerk | Return the health projection |
| `POST` | `/v1/grants/verify` | grants clerk | Verify last name and return alive/address status |
| `POST` | `/v1/police/verify` | police clerk | Verify date of birth and alive status |
| `GET` | `/v1/audit/my-department` | audit auditor | Read department audit events |

National IDs are supplied in request bodies, checked for the documented
13-digit format/check digit and protected through Vault Transit. `District` is
used as the grants province field, as configured for this schema. Electoral
roll routes are not mounted because the current address model has no `ward_code`.
This is intentionally not generic CRUD: deletes and direct writes to audit,
outbox, delivery, backup, biometric or authorization tables are not exposed.

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

Birth/death write operations, electoral routes and event synchronisation are
not implemented. Birth/death writes require the design's stored procedures;
electoral routes require `ward_code`, which the preserved address model does
not contain. The currently mounted API is the protected subset listed above,
not unrestricted CRUD over the database tables.
