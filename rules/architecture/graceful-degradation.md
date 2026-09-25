# Graceful Degradation

## Focus
- Optional subsystem initialization (rule loaders, plugin registries, telemetry exporters) MUST NOT crash the process on failure. Log a warning and continue with defaults.
- External service calls MUST have timeouts. Missing `context.WithTimeout` or `http.Client.Timeout` on outbound calls is a finding.
- Circuit breakers or retry-with-backoff SHOULD be used for calls to LLM providers and VCS APIs.
- Feature flags for new functionality SHOULD have a safe default (disabled) so rollback is a config change, not a deploy.

## DO NOT Flag
- Required subsystem failures (database connection, Temporal connection) that correctly prevent startup.
- Test helpers that panic on setup failure.
