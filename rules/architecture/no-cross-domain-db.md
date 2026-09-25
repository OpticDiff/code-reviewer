# no-cross-domain-db

## Focus
Microservices must communicate strictly across network boundaries via ConnectRPC
or asynchronous domain events over AutoMQ. Direct cross-domain SQL joins or queries
against another service's private operational tables are strictly forbidden.

## DO NOT Flag
As established in Section 7 of the Guiding Principles,
high-volume, non-PHI clinical knowledge (SNOMED CT, ICD-10-CM, RxNorm crosswalks)
resides in the global terminology.* schema and is permitted for read-only queries
across all services and tenant contexts. Writes to terminology.* remain strictly
restricted to Atlas Operator deployments.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/database-access
