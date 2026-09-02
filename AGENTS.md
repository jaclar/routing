# Repository Architecture & Engineering Guidelines

## 1. Port & Network Allocation Standard
Every service MUST expose a unique port strictly within the 4000–5000 range:
- `4000`: Frontend Web App (React / TypeScript / Vite / Leaflet)
- `4001`: VPP Service (Python / FastAPI / Aero-Hydro Solver)
- `4080`: Routing Service (Go / Isochrone Engine)
- `4081`: Meteo Weather Service (Go / Open-Meteo API & NWP Ingestion)

Any new microservice must be assigned an unused port in this range and registered in:
1. `docker-compose.yml` (production container env & network)
2. `docker-compose.dev.yml` (host port bindings)
3. `Caddyfile` (reverse proxy edge routing)
4. `services/frontend/vite.config.ts` (local dev server proxies)

---

## 2. Go Microservice Project Structure
All Go microservices under `services/<service-name>/` must follow the standard Go layout:
```
services/<service-name>/
├── cmd/
│   ├── api/main.go            # HTTP server entrypoint
│   └── worker/main.go         # CLI / daemon entrypoint (if applicable)
├── internal/                  # Private domain packages (not importable outside service)
│   ├── model/                 # Data structures, domain types, unit conversions
│   ├── driver/                # Upstream providers / external integrations
│   ├── query/ or api/         # Request handling & serializers
│   └── store/ or zarr/        # Data access and storage layer
├── Dockerfile                 # Standard multi-stage alpine build
├── go.mod                     # Go 1.22+ module definition
└── README.md
```

---

## 3. Go Coding & Architecture Rules
- **Pure Go First**: Prefer pure Go decoders, compressors, and parsers (e.g. `grib2`, `zstd`, `bzip2`) to avoid CGo build friction and ensure smooth multi-arch Docker compilation (`amd64` / `arm64`).
- **HTTP Routing**: Use `github.com/go-chi/chi/v5` with standard middleware (Logger, Recoverer, CORS, RequestID, Timeout).
- **Health Check**: Every HTTP service MUST expose `GET /health` returning `{"status": "ok", "service": "<name>"}`.
- **Configuration**: All ports, data paths, and upstream URLs must read from environment variables with fallback defaults (e.g., `PORT=4080`, `DATA_DIR=/data/store`, `VPP_SERVICE_URL=http://localhost:4001`, `METEO_SERVICE_URL=http://localhost:4081`).
- **Error Handling**: Wrap errors using `fmt.Errorf("...: %w", err)`; never allow uncaught panics in HTTP handlers.
- **Testing**: Every package in `internal/` must include unit tests (`*_test.go`) runnable via `go test -v ./...` without external network dependencies.

---

## 4. Docker & Deployment Conventions
- **Multi-stage Dockerfile**:
  - Builder stage: `golang:1.22-alpine` with `CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s"`
  - Runtime stage: `alpine:3.19` with `ca-certificates` and `tzdata`.
- **Docker Compose**:
  - Connect all microservices to `routing-network` bridge network.
  - Expose persistent volumes using the naming pattern `sailboat_<service>_data`.
- **Caddy Ingress**:
  - Update `Caddyfile` with appropriate route handles targeting `<service-container>:<port>`.

---

## 5. Development Workflows
- `pnpm dev:backend`: Starts all backend containers (`routing-service:4080`, `vpp-service:4001`, `meteo-service:4081`).
- `pnpm dev`: Runs frontend with Vite HMR on `http://localhost:4000`.
- `pnpm dev:docker`: Starts full container stack with live file sync.

---

## 6. Git & Version Control Guidelines
- **Explicit Commits Only**: NEVER run `git commit` or commit any changes unless the user explicitly asks you to commit.
- **Granular Commits by Service & Feature**: When asked to commit, ALWAYS make separate, focused commits grouped by **service** (e.g., `services/routing`, `services/frontend`, docker/root configs) and **feature/change**, never bundling all workspace changes into one huge commit.

