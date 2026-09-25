# enforce-tenant-db-context

## Focus
All application queries and database interactions must route through the platform's
tenant transaction helper: platform/sdk/db.TenantTx(ctx).
Direct usage of raw *sql.DB, *sql.Tx, or pgx.Conn handles without tenant context
is strictly prohibited in domain and worker logic.
Unscoped database connections risk executing against the default public schema
or leaking records across tenant_<slug> PostgreSQL boundaries, violating HIPAA
physical tenancy requirements.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/tenant-isolation
