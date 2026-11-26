# Goliath Public Registry

Goliath is a modular registry API with separate Go backend and Vue frontend.
The Vue app provides a system administration dashboard and role-scoped
department workspaces.

## Repository layout

```text
.
├── api/                      # Go API, MySQL models, auth and feature modules
│   ├── cmd/api/
│   ├── internal/
│   │   ├── auth/             # App-managed accounts, password hashing, sessions
│   │   ├── metrics/          # HTTP and Go runtime telemetry
│   │   └── modules/
│   │       ├── admin/        # Login, admin, user and department endpoints
│   │       ├── health/
│   │       ├── identity/
│   │       └── registry/
│   ├── .env.example
│   ├── go.mod
│   └── go.sum
├── frontend/                 # Vue 3 + TypeScript + Vite
│   └── src/
│       ├── components/       # Shared layout, cards and status components
│       ├── services/         # API client and department service definitions
│       ├── stores/           # In-memory authenticated session
│       └── views/            # Admin and department pages
├── docker-compose.yml        # Local MySQL dependency
└── README.md
```

## Local development

### 1. Configure the API

Copy `api/.env.example` to `api/.env`. Set database credentials, Vault Transit
configuration, and a strong initial administrator password of at least 15
characters. Keep `.env` files out of source control.

The configured Vault Transit mount must contain:

- The encryption key named by `VAULT_NID_ENCRYPTION_KEY`.
- An HMAC key named by `VAULT_NID_HMAC_KEY`.

For a local MySQL container, start it from the repository root with:

```bash
docker compose --env-file api/.env up -d mysql
```

The database port is bound to loopback. Vault is intentionally managed
separately; configure a local or remote Transit service, using HTTPS except
for loopback development.

### 2. Run the API

```bash
cd api
go test ./...
go run ./cmd/api
```

The API listens on `http://127.0.0.1:8080` by default. It runs schema
auto-migrations at startup. On the first startup only,
`BOOTSTRAP_ADMIN_USERNAME` and `BOOTSTRAP_ADMIN_PASSWORD` create the initial
global administrator and an `ADMIN` system department. The password must be a
unique passphrase of at least 15 characters and must not contain the username.
The bootstrap administrator must change this password at first sign-in. Once
the administrator exists, remove the bootstrap credentials from the
environment; the application will not reset an existing password from those
values.

The API is deliberately restricted to a loopback bind until HTTPS/mTLS
deployment is configured.

### 3. Run the Vue frontend

In another terminal:

```bash
cd frontend
npm install
npm run dev
```

Open <http://127.0.0.1:5173>. Vite proxies `/auth`, `/admin`, `/health`, `/api`
and `/v1` requests to `API_PROXY_TARGET`, which defaults to
`http://127.0.0.1:9090`. To use another API address, copy
`frontend/.env.example` to `frontend/.env.local` and change `API_PROXY_TARGET`;
restart the Vite dev server after changing it. The frontend keeps the
short-lived bearer token in memory only; refreshing the page requires signing
in again.

Build and type-check the frontend with `npm run build`.

## Authentication and account provisioning

There is no public registration flow. The bootstrap administrator signs in and
creates staff accounts from **Administration → User directory**. New users are
assigned to an active department and a clerk, supervisor or auditor role. The
API generates a temporary password, returns it once to the administrator, and
requires the user to change it at first sign-in. Account deactivation revokes
all existing sessions. Administrators can also create, edit and deactivate
departments.

Passwords are hashed with Argon2id. Passwords must be at least 15 characters,
must not contain the account username, and may include spaces. Prefer a unique
passphrase and a password manager; avoid common or compromised passwords and
do not share credentials in an unapproved channel. Password changes revoke
other sessions and issue a replacement session to the current user.

Sessions use random opaque bearer tokens with an eight-hour expiry; only a
SHA-256 token digest is stored in MySQL. Global administrators can view
administrative pages and system metrics. Department users are restricted by
their assigned department and minimum role. Administrative actions and
department registry operations are written to the per-node audit hash chain.

## Admin dashboard and metrics

The administrator dashboard reads `GET /admin/metrics` and displays:

- API health, service uptime, total requests, average response time and HTTP
  status counts.
- Go runtime process CPU time, available CPU cores, goroutines, heap usage,
  cumulative allocation and GC cycles.
- MySQL connection pool availability and wait statistics.

Metrics are collected in-process and are reset when the API process restarts.
The authenticated metric response checks database connectivity as well as
process health. This is operational telemetry, not a replacement for durable
monitoring or an external alerting system.

## API endpoints

| Method | Path | Access | Purpose |
|---|---|---|---|
| `POST` | `/auth/login` | Public | Sign in with an administrator-provisioned account |
| `POST` | `/auth/change-password` | Authenticated | Change password, including first-login reset |
| `POST` | `/auth/logout` | Authenticated | Revoke the current session |
| `GET` | `/admin/metrics` | Global admin | View service, runtime and database health |
| `GET` | `/admin/users` | Global admin | List provisioned accounts |
| `POST` | `/admin/users` | Global admin | Provision a department account |
| `PATCH` | `/admin/users/{id}` | Global admin | Activate or deactivate a user |
| `GET` | `/admin/departments` | Global admin | List departments |
| `POST` | `/admin/departments` | Global admin | Create a department |
| `PATCH` | `/admin/departments/{code}` | Global admin | Edit or deactivate a department |
| `GET` | `/health` | Public | Process and database health |
| `GET` | `/api/endpoints` | Public | Discover mounted API routes |
| `POST` | `/v1/identity/persons` | Identity supervisor | Register a person |
| `POST` | `/v1/identity/persons/lookup` | Identity clerk | Look up a person |
| `PATCH` | `/v1/identity/persons/{id}/address` | Identity clerk | Change a current address |
| `POST` | `/v1/identity/marriages` | Identity supervisor | Record a marriage |
| `POST` | `/v1/health/persons/lookup` | Health clerk | Read the health person projection |
| `POST` | `/v1/grants/verify` | Grants clerk | Verify a grants identity |
| `POST` | `/v1/police/verify` | Police clerk | Verify a police identity |
| `GET` | `/v1/audit/my-department` | Audit auditor | Read department audit events |

National IDs are checked and encrypted/indexed through Vault Transit. This is
purposeful API access, not unrestricted CRUD; sensitive tables such as audit,
biometric, backup and authorization data are not exposed as generic resources.

## Configuration reference

| Variable | Required | Notes |
|---|---|---|
| `APP_NAME` | No | Service name reported in health responses |
| `APP_ENV` | No | `production` selects JSON logs |
| `LOG_LEVEL` | No | `debug`, `info`, `warn` or `error` |
| `SERVER_HOST` | No | Defaults to `127.0.0.1`; non-loopback HTTP is rejected |
| `SERVER_PORT` | No | Defaults to `8080` |
| `DB_HOST`, `DB_PORT` | No | Defaults to `127.0.0.1:3306` |
| `DB_USER`, `DB_NAME` | Yes | MySQL application credentials/schema |
| `DB_PASSWORD` | No | MySQL application password |
| `DB_CHARSET` | No | Defaults to `utf8mb4` |
| `MYSQL_ROOT_PASSWORD` | Compose only | Local MySQL container root password |
| `NODE_ID` | Yes | Audit hash-chain node ID, max 20 characters |
| `BOOTSTRAP_ADMIN_USERNAME` | First startup | Initial administrator username |
| `BOOTSTRAP_ADMIN_PASSWORD` | First startup | Strong initial administrator password |
| `VAULT_ADDR`, `VAULT_TOKEN` | Yes | Vault Transit service address and token |
| `VAULT_TRANSIT_MOUNT` | No | Defaults to `transit` |
| `VAULT_NID_ENCRYPTION_KEY` | Yes | Transit encryption key name |
| `VAULT_NID_HMAC_KEY` | Yes | Transit HMAC key name |

## Validation

Backend checks:

```bash
cd api
go test ./...
go vet ./...
```

Frontend checks:

```bash
cd frontend
npm run build
```
