# state-reconciled-idempotency

## Focus
Provisioning operations, webhook registrations, and infrastructure mutations
must implement true state reconciliation rather than existence-check skipping.
If an entity already exists, update and reconcile its desired state (e.g. via PUT)
rather than skipping and leaving stale credentials or endpoints.

## DO NOT Flag
Standard exclusions apply.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/architecture/idempotency
