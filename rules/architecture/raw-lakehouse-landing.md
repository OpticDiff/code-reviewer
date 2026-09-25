# raw-lakehouse-landing

## Focus
Clinical and operational data ingestion feeds must land inbound messages bit-for-bit
into Apache Iceberg before performing lossy transforms, filtering, or domain enrichment.
Follow the "First-Write" principle: the raw byte buffer from the network socket or
stream reader must be appended directly to the Iceberg landing layer before downstream
parsing or schema normalization is invoked.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/lakehouse-ingestion
