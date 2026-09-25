# temporal-nexus-boundary

## Focus
Workers providing public, cross-boundary asynchronous operations must expose them
as Temporal Nexus Services (via github.com/nexus-rpc/sdk-go and w.RegisterNexusService)
rather than requiring callers to directly enqueue workflows onto private task queues.

Flag violations including:
- Client code outside the worker's pod importing internal workflow types or directly
  starting workflows on private task queues.
- Workers with public API proto endpoints that don't provide a corresponding
  Nexus Service registration in internal/nexus/ or internal/workflow/.
- Leaking internal activity types across service boundaries.

## DO NOT Flag
Internal child workflows, saga orchestrators, and private utility
workflows are exempt and must not be exposed as Nexus operations.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/temporal-nexus-boundaries
