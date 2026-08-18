# HTTP Deployment Posture

**Audience:** operators running the Cartesi rollups-node in production.
**Applies to:** releases that include the HTTP hardening package (2.0.0-alpha.12 and later).

This document describes the HTTP-facing surfaces of the node, how they are
protected in-process, and the deployment posture operators are expected to
provide around them. The node assumes a **trusted network boundary** — the
in-process controls are a defense-in-depth layer, not a substitute for
operator-side network policy.

## The three HTTP surfaces

| Surface | Default address | Purpose | Per-request cost |
| --- | --- | --- | --- |
| **Telemetry** (`/livez`, `/readyz`) | `:10000` | Orchestrator health checks | Trivial — a boolean check and a short response |
| **JSON-RPC API** (`/rpc`) | `:10011` | Read-only query interface | Up to 1 MiB body; one list operation, or a batch with a cumulative list limit of 10000 items; DB queries |
| **Inspect** (`/inspect/{dapp}`) | `:10012` | Machine state query without advancing | Up to 2 MiB body, Cartesi Machine fork + execution |

Telemetry is cheap by design — orchestrators (Kubernetes, Docker,
systemd health checks) hammer it intentionally. JSON-RPC and inspect
are the expensive surfaces and are the targets of the hardening package.

## Bind defaults

All three services bind to `:PORT` (all interfaces) by default. This is
required for Docker container port publishing and for typical reverse-proxy
front-ending patterns where the node listens on an in-container interface
the proxy can reach.

**On startup, the node logs a warning** for every HTTP service that binds
to an unspecified address (`:PORT`, `0.0.0.0:PORT`, or `[::]:PORT`):

```text
WRN HTTP service bound to all interfaces; restrict access via firewall or reverse proxy
    service=inspect addr=:10012
```

This warning is expected under Docker and Compose. In bare-metal
deployments without a reverse proxy, consider overriding the addresses to
loopback only:

```bash
CARTESI_INSPECT_ADDRESS=127.0.0.1:10012
CARTESI_JSONRPC_API_ADDRESS=127.0.0.1:10011
CARTESI_TELEMETRY_ADDRESS=127.0.0.1:10000
```

## Recommended deployment posture

The node's HTTP surfaces are designed to sit **behind a reverse proxy**
(nginx, Caddy, Traefik, Envoy, or a cloud API gateway). The proxy is
expected to provide:

- **TLS termination.** The node speaks plain HTTP only.
- **Per-IP rate limiting.** The node has no IP-awareness; the proxy's
  `limit_req`/`limit_conn`-equivalent primitives own that responsibility.
- **Authentication and authorization.** The node's HTTP endpoints are
  unauthenticated; do not expose them to untrusted clients without a
  proxy that enforces auth.
- **Connection and request caps at the network layer.** The in-process
  admission control (described below) is a second line of defense, not
  a replacement for proxy-level limits.

For internal-only deployments without external exposure:

- Bind to loopback or a private-network interface.
- Use a firewall (`iptables`, `nftables`, cloud security groups) to
  restrict source addresses.
- Treat `inspect` as a sensitive execution surface — it can run machine
  code on demand and should stay internal whenever possible.

**Browser exposure.** CORS is **disabled by default** on both JSON-RPC
and inspect. No `Access-Control-Allow-Origin` header is emitted unless
the operator explicitly configures an origin allowlist via
`CARTESI_JSONRPC_CORS_ALLOWED_ORIGINS` or
`CARTESI_INSPECT_CORS_ALLOWED_ORIGINS`. When configured, only
exact-match origins are reflected — wildcard (`*`) is never used.

For production browser exposure, prefer handling CORS at the reverse
proxy (nginx, Caddy, Envoy) rather than in the node.

Note that `http://localhost:3000` and `http://127.0.0.1:3000` are
**distinct origins** in the browser's same-origin policy. If you bind
the node to `127.0.0.1` but your frontend dev server runs at
`http://localhost:3000`, you must allowlist the origin the browser
sees (e.g. `http://localhost:3000`), not the bind address.

CORS configuration is read at startup and is not reloadable. Changing
allowed origins requires a process restart.

## Timeout baselines

Each HTTP surface uses a named preset from `pkg/service` (exported as
`DefaultInspectOptions`, `DefaultTelemetryOptions`, `DefaultJSONRPCOptions`).
All five fields are set on every server.

| Field | Inspect | Telemetry | JSON-RPC |
| --- | --- | --- | --- |
| `ReadHeaderTimeout` | 10s | 5s | 10s |
| `ReadTimeout` | 30s | 10s | 30s |
| `WriteTimeout` | 600s | 10s | 30s |
| `IdleTimeout` | 60s | 60s | 60s |
| `MaxHeaderBytes` | 64 KiB | 16 KiB | 64 KiB |

**Inspect `WriteTimeout` = 600s** is a process-health backstop. The actual
per-request deadline is set structurally in `Inspector.ServeHTTP` as
`InspectMaxDeadline + 30s` (the 30-second headroom covers response
serialization). The HTTP `WriteTimeout` prevents leaked goroutines from
holding a connection forever but never participates in normal request
lifecycle.

### Per-request deadline enforcement

Each inspect request receives a `context.WithTimeout` set to the
application's `InspectMaxDeadline + 30s` (the 30 seconds cover JSON
response serialization and wire delivery). This deadline is set in
`Inspector.ServeHTTP` after resolving the application, before invoking
the Cartesi Machine. The machine manager sets a nested
`context.WithTimeout(ctx, InspectMaxDeadline)` for the machine execution
itself; Go context nesting means the shorter deadline always wins.

This structural approach eliminates the need for coordinating
`WriteTimeout` with `InspectMaxDeadline` — operators configure per-app
deadlines without worrying about HTTP-layer timeouts.

## Admission control

Both expensive HTTP surfaces (JSON-RPC and inspect) have an **in-process
concurrency gate** that fails fast when the number of in-flight requests
exceeds a configured limit. This bounds goroutine count, per-request
memory, and backend contention even under a flood.

### Configuration

| Env var | Default | Scope |
| --- | --- | --- |
| `CARTESI_INSPECT_MAX_INFLIGHT` | `64` | Inspect service only |
| `CARTESI_JSONRPC_MAX_INFLIGHT` | `64` | JSON-RPC service only |

Each surface has its **own independent budget**. A flood on one
surface cannot starve the other. Telemetry is never gated — its
per-request cost is too low to justify admission.

A value of `0` disables admission on that surface. Backpressure then
falls back to:

- Inspect: the per-application Cartesi Machine semaphore (blocking, not
  fail-fast; deeper in the request path).
- JSON-RPC: the PostgreSQL connection pool (blocking).

### JSON-RPC batch work budget

Admission counts HTTP requests, while a JSON-RPC batch can contain up to 100
operations. To keep one admitted batch from buying substantially more row-fetch
work than one maximal list request, the service applies a protocol-level budget
before dispatch:

- The sum of the effective `limit` values across all list entries in a batch
  must not exceed 10000.
- An omitted or zero `limit` counts as the default of 50. A value above the
  per-list maximum is capped to 10000 before it is added.
- If the sum exceeds 10000, the whole batch is rejected before any handler or
  database query runs. The response is one JSON-RPC error object with code
  `-31004` and message `Batch list item limit exceeded`.
- Non-list entries do not consume this work budget. A single request retains
  the existing per-list maximum of 10000.

This restores the row-fetch bound that existed before batch support: one
admission slot can fetch at most as many rows as one maximal list call. It does
not bound `COUNT(*)` cost, which is independent of `limit`. It also does not
meter `offset` traversal cost; that cost is bounded by the size of the filtered
set rather than by the numeric `offset` value, but PostgreSQL may still have to
scan and discard the matching rows before the requested page. Selective
filters, the pending-output partial index, proxy rate limiting, and PostgreSQL
capacity planning remain important.

### Rejection semantics

When admission rejects a request:

```text
HTTP/1.1 503 Service Unavailable
Content-Type: text/plain; charset=utf-8
X-Content-Type-Options: nosniff
Retry-After: 1-3 (jittered)

service at capacity (request_id=<uuid>)
```

- **Silent by default** — the rejection is not logged per-request
  (logging every rejection during a flood would amplify the flood).
- The `Retry-After` header carries a jittered value in `[1, 3]` seconds
  to desynchronize retrying clients and prevent thundering-herd pile-ups.
  Clients and proxies may or may not honor it.
- Operators observe saturation via the monotonic `Rejected()` counter
  exposed on the `SemaphoreAdmission` instance. Wiring into a metrics
  backend is out of scope for this release; see the rollups-node bug
  taxonomy for when that becomes urgent.

Note that the 503 response is `text/plain`, not a JSON-RPC error
envelope. JSON-RPC 2.0 client SDKs will treat this as a transport-level
error, not a protocol-level one. This is deliberate: admission rejection
happens before the request reaches the JSON-RPC handler, so a transport
error is the correct signal.

### Tuning guidance

The defaults (64 in-flight per surface) are conservative and fit a
single-node deployment on typical hardware. Consider raising the limit
when:

- The reverse proxy is already doing per-IP rate limiting and the
  admission gate is hitting the ceiling on legitimate traffic.
- DB / machine capacity can sustain more concurrent work and the
  `Rejected()` counter is non-trivial under normal load.

Consider lowering when:

- Memory pressure from buffered request bodies is observable.
- Concurrent machine forks are slowing legitimate inspects to the point
  of cascading timeouts.

## Request bodies

| Surface | Cap | Enforcement |
| --- | --- | --- |
| Telemetry | — | No request body |
| JSON-RPC | **1 MiB** | `http.MaxBytesReader` in the handler |
| Inspect | **2 MiB** | `http.MaxBytesReader` in the handler (matches the Cartesi Machine CMIO RX buffer) |

Oversized bodies are rejected with:

```text
HTTP/1.1 413 Request Entity Too Large
Content-Type: text/plain; charset=utf-8

Payload too large
```

The connection is then force-closed by the stdlib, so clients cannot
pipeline additional requests on the same connection. This behavior
depends on the internal `responseWriterTap.Unwrap()` cooperating with
`http.MaxBytesReader`; see the hardening v3 plan for the design note.

**Worst-case body buffer memory under saturation.**
Each admitted request pins its body buffer for the full request lifetime
(up to `InspectMaxDeadline + 30s` for inspect (typically ~210s with the default 180s deadline), 30s for JSON-RPC). At default concurrency this
means `CARTESI_INSPECT_MAX_INFLIGHT × 2 MiB = 128 MiB` for inspect and
`CARTESI_JSONRPC_MAX_INFLIGHT × 1 MiB = 64 MiB` for JSON-RPC. Operators
should size process RAM headroom accordingly, on top of machine state,
database connections, and other working memory.

## PostgreSQL pool sizing

The node uses `pgxpool` for database access. The pool's default maximum
connection count is `max(4, runtime.NumCPU())` unless overridden in the
connection URL. On a typical 4-core container this means 4 connections
shared across every service.

**Rule of thumb:** set `pool_max_conns` to at least
`CARTESI_JSONRPC_MAX_INFLIGHT + steady-state writer services`. In
standalone mode the writer services (EVM Reader, Advancer, Validator,
Claimer, PRT, plus overhead) account for roughly 6-8 connections, so a
conservative floor is `64 + 8 = 72`. Override via the connection URL:

```bash
CARTESI_DATABASE_CONNECTION="postgres://user:pass@db:5432/rollups?pool_max_conns=72&pool_max_conn_lifetime=30m"
```

**Fail-fast vs. fail-slow.** Admission rejects are visible and
immediate (503 with `Retry-After`). Exceeding the pool capacity is
invisible and slow — handlers that passed admission block inside
`pgxpool.Acquire` while holding admission permits, causing latency
degradation with no obvious signal to the operator. Sizing the pool to
match the admission limit prevents this silent backpressure. A future
release may emit a startup warning when the sum of admission limits
exceeds the configured pool size.

Note that the inspect surface also queries the database (one
`GetApplication` call per request) and shares the same underlying pool,
so both hardened HTTP surfaces compete for connections.

## Request IDs

Every response from inspect, JSON-RPC, and telemetry includes an
`X-Request-ID` header. The middleware enforces:

- **Validation:** upstream `X-Request-ID` is trusted only if it matches
  `^[A-Za-z0-9._:=/+-]{1,128}$`. Values outside that charset or longer
  than 128 characters are **discarded** and a fresh UUIDv4 is generated
  in their place. This prevents log injection via `\n` / `\r` and caps
  header cardinality.
- **Generation:** when no upstream ID is present (or the upstream value
  was rejected), the node generates a UUIDv4 via `github.com/google/uuid`.
- **Propagation:** the chosen ID is placed on the request context and
  echoed on the response `X-Request-ID` header. Error log lines from the
  handler include the ID as a structured field.

This lets operators correlate a single request across:

- The reverse proxy's access log (if it assigns IDs upstream).
- The rollups-node's structured log output.
- The client's error response (see next section).

## Internal errors

When a handler hits an unexpected error that it can't express as a
domain-level status code, it responds with:

```text
HTTP/1.1 500 Internal Server Error
Content-Type: text/plain; charset=utf-8

Internal server error (request_id=<id>)
```

The original Go `error` value is **never** written to the response body.
Its full content — message, wrapped chain, stack if available — is
logged at ERR level with the request ID as a structured field. Operators
triage a user's 500 report by grepping logs for the request ID they
reported.

As with admission 503s, the 500 response is `text/plain`, not a JSON-RPC
error envelope. JSON-RPC client SDKs will surface these as transport
errors, which is the correct signal for a server-side fault.

## Panic recovery

Every HTTP handler chain is wrapped in a panic-recovery middleware:

- **If the handler panics before any byte has been written**, the
  middleware catches the panic, logs the value and stack trace at ERR
  level with the request ID, and writes a generic 500 (same format as
  "Internal errors" above).
- **If the handler panics after bytes have been flushed**, the
  middleware cannot safely write a 500 without producing a corrupt
  response (stitching a `500 Internal Server Error` onto a started 200
  would lie to the client and trigger Go's "superfluous WriteHeader"
  warning). Instead, the middleware re-panics with `http.ErrAbortHandler`
  — the stdlib's documented sentinel for "abort this connection
  silently". The client observes a truncated response and connection
  drop, which is the honest signal.
- **Panics whose value is already `http.ErrAbortHandler` are re-panicked
  unchanged.** This preserves the stdlib contract for handlers that
  intentionally use the sentinel to abort without logging.

## Known limitations

1. **Two-layer admission on inspect.** Inspect requests pass through two
   independent concurrency gates: the HTTP-global admission gate (default
   64 in-flight, configured via `CARTESI_INSPECT_MAX_INFLIGHT`) and a
   per-application machine semaphore (`MaxConcurrentInspects`, default
   10). Both gates are fail-fast (`TryAcquire`): when either is full the
   request is rejected immediately with 503 and the caller's admission
   permit is released. This means one saturated application does **not**
   starve others — its excess requests fail at the per-app gate and free
   HTTP-global capacity for other apps. Operators should be aware that
   both layers return 503; the HTTP-global gate includes a `Retry-After`
   header while the per-app gate does not.

2. **Single-replica assumption.** The admission budget (default 64 per
   surface) is per-process. Multi-replica deployments behind a load
   balancer have an effective budget of `replicas × CARTESI_*_MAX_INFLIGHT`.
   The default 64 is sized for a single-node deployment on typical
   hardware; operators should not assume it represents a global limit.

## Non-goals

The following are explicitly **out of scope** for the HTTP hardening
package. If you need them, add them at the reverse proxy or via
follow-up work.

- TLS termination inside the node.
- Authentication / authorization on HTTP endpoints.
- Per-IP rate limiting.
- Per-application fairness inside the admission gate (the gate is
  global per HTTP surface).
- Global cross-service admission (inspect and JSON-RPC have independent
  budgets).
- Admission on telemetry.
- Wiring `SemaphoreAdmission.Rejected()` into a metrics backend.
- Flipping bind defaults to loopback.
- Exposing `net/http/pprof`.

## Related documentation

- `docs/config.md` — generated reference for every `CARTESI_*`
  environment variable, including `CARTESI_INSPECT_MAX_INFLIGHT` and
  `CARTESI_JSONRPC_MAX_INFLIGHT`.
