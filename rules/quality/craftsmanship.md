# Craftsmanship Rules

# bounded-io-reads

## Focus
HTTP response bodies, file streams, and network buffers must never be read
using unbounded io.ReadAll without a maximum byte limit.
Always bound reads with io.LimitReader(resp.Body, maxBytes) or bounded streaming
decoders to prevent memory exhaustion (OOM) and DoS attacks.

## DO NOT Flag
Standard exclusions apply.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/architecture/bounded-io

---

# resilient-composite-pipeline

## Focus
Composite workflows that execute multiple non-fatal or optional setup steps
should accumulate errors rather than bailing out at the first minor failure.
Execute independent setup steps, collect errors with errors.Join or diagnostic slices,
and return partial success notices so downstream steps remain operational.

## DO NOT Flag
Standard exclusions apply.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/architecture/pipeline-resilience

---

# valid-api-contracts-and-retries

## Focus
External API interactions and retry loops must inspect HTTP status codes
before initiating retries. Never retry blindly on 4xx client errors (400, 401, 403, 404).
Only retry on transient conditions (429 rate limits, 5xx server errors, connection resets).

## DO NOT Flag
Standard exclusions apply.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/architecture/retry-hygiene

---

# propagate-request-context

## Focus
Incoming request context (ctx context.Context) must be consistently propagated
through all downstream database queries, HTTP calls, RPC invocations, and activity steps.
Do not substitute context.Background() or context.TODO() inside non-root request
handlers or Temporal activities, as this severs deadline propagation, tracing spans,
and tenant cancellation signals.

## DO NOT Flag
Standard exclusions apply.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/architecture/context-propagation

---

# slog-structured-fields

## Focus
Application logging must utilize log/slog with structured key-value attributes
(e.g., slog.InfoContext(ctx, "event processed", "tenant_id", tid, "duration_ms", ms))
rather than unstructured log.Printf, fmt.Println, or string interpolation.

## DO NOT Flag
Never log sensitive clinical or identity attributes
(patient_name, ssn, mrn, dob, auth_token, password). Mask identifiers or log
opaque UUIDs only.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/observability/logging-standards

---

# connect-status-codes

## Focus
ConnectRPC and gRPC service implementations should return typed Connect status
codes (e.g., connect.NewError(connect.CodeNotFound, ...), connect.CodeInvalidArgument,
connect.CodeFailedPrecondition) rather than returning raw Go errors directly.
Explicit status code mapping prevents leaking internal database errors to clients
and enables upstream callers to make deterministic retry decisions.

## DO NOT Flag
Standard exclusions apply.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/architecture/connectrpc-errors

---

