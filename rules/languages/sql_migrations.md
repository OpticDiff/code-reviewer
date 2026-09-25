# SQL Migrations Review Checklist

## Focus
- **Safety**: DDL safety (DROP without expand-contract).
- **Constraints**: NOT NULL without DEFAULT.
- **Performance**: Large table ALTER risks, index creation without CONCURRENTLY.
- **Completeness**: Missing DOWN migration.

## DO NOT Flag
- SQL formatting.
- Comment style.
