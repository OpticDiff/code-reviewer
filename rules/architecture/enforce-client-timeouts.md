# enforce-client-timeouts

## Focus
HTTP clients, gRPC dialers, and external network connections must configure explicit
request timeouts and deadline policies.
Usage of Go's http.DefaultClient or unconfigured &http.Client{} without Timeout is
prohibited, as stalled upstream connections will leak goroutines and freeze worker slots
under network partitions or external outages.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/http-client
