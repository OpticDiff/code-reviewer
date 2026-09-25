# no-hardcoded-numeric-identifiers

## Focus
Do not hardcode internal numeric database, forge, or identity IDs (such as
GitLab numeric User IDs, Group IDs, or Project IDs like UID 42502232 or
GID 138384595) in source code or configuration files.

Numeric IDs are environment-specific, brittle, and fail silently when entities
are deleted, restored, or migrated across environments. Always reference entities
by stable names or paths and resolve IDs dynamically via API lookups.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/identifiers
