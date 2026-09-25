# db-migration-safety

## Focus
Audit SQL migration files for unsafe operations. Flag:
- DROP TABLE or DROP COLUMN on any schema without expand-and-contract
  staging (must have separate expand and contract migrations)
- Renaming columns without a compatibility alias period
- Adding NOT NULL columns without DEFAULT values
- Large table alterations without online DDL considerations
- Missing DOWN migration for reversibility

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/migration-safety
