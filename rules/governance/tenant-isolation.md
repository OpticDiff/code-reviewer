# tenant-isolation

## Focus
Enforce schema-per-tenant architecture. All database queries must use
database.TxFromContext(ctx) or TenantTx to obtain a tenant-scoped
transaction. Direct SQL queries without tenant context, raw *sql.DB
usage bypassing the tenant transaction layer, or cross-tenant data
access patterns are violations.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/tenant-isolation
