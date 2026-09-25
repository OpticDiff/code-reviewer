# atlas-destructive-migration

## Focus
Database migrations must be declarative Atlas HCL or versioned SQL migrations
satisfying zero-downtime safety policies (destructive.error = true).
Flag non-backward-compatible schema operations including:
- Dropping tables or columns without a multi-phase deprecation lifecycle
- Renaming columns or tables directly without an expand/contract adapter
- Adding non-null columns without default values to existing tables (table lock risk)

## DO NOT Flag
Tables or schemas introduced and modified within the exact same PR
are permitted, as are ephemeral dev schemas. When performing the contract (drop)
phase of a column migration, reference the prior expand PR in the MR description.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/database-migrations
