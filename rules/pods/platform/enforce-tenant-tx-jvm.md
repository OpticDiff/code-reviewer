# enforce-tenant-tx-jvm

## Focus
Database interactions in JVM microservices and workers must use the platform SDK's
tenant-scoped transaction block: tenantTx { ... }.
Direct instantiation of java.sql.DriverManager, raw DataSource.getConnection() bypassing
the tenant context, or un-scoped Hibernate/JPA sessions without schema/search_path
isolation are strictly prohibited in domain code.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/jvm-tenant-tx
