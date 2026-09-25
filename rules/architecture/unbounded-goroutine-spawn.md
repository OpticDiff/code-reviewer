# unbounded-goroutine-spawn

## Focus
Goroutines spawned in request-processing paths, ConnectRPC handlers, or background
workers must bind to structured concurrency primitives: sync.WaitGroup, errgroup.Group,
or a bounded worker pool with context cancellation awareness.
Raw go func() invocations without lifecycle tracking or concurrency limits risk memory
exhaustion (OOM) and socket depletion under burst traffic in multi-tenant environments.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/concurrency-guidelines
