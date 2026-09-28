# single-workflow-per-worker

## Focus
Each Temporal worker should register exactly ONE workflow. Flag any file
that calls `RegisterWorkflow()` more than once, or any `register.go` /
`register_all.go` that wires multiple workflows to the same worker.

This enforces the one-worker-one-workflow constraint:
- Each workflow gets its own binary, task queue, and scaling profile
- Prevents workflow sprawl inside a single worker
- Makes deployment, rollback, and resource tuning per-workflow

## Detect
- Multiple `RegisterWorkflow(...)` calls in the same function or file
- Multiple `workflow.Context` functions registered to the same `worker.Registry`
- Comments like "scaffold" or "example" on workflow registrations (dead code)

## DO NOT Flag
- Multiple `RegisterActivity(...)` calls — activities can be shared
- Test files that register workflows for testing purposes
- Files that explicitly document a MAX_WORKFLOWS override

## Severity
High

## Suggestion
Split into separate workers, each with its own binary, task queue, and
Kubernetes deployment. If temporary co-location is needed, add a
`MAX_WORKFLOWS` override to the litmus check with a tracking issue.
