# enforce-threadlocal-cleanup-jvm

## Focus
All JVM/Kotlin code utilizing ThreadLocal or TenantContext.withTenant() must guarantee
immediate context removal via a mandatory finally { TenantContext.remove() } block
or Kotlin's use { } idiom.
In pooled thread environments (Netty, gRPC, HikariCP) and Java 21 virtual threads,
failing to clear ThreadLocal context results in thread-bleed, leaking tenant identity
and patient data across unrelated requests. This is a critical violation of HITRUST 01.c
and multi-tenant healthcare isolation.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/jvm-threadlocal-cleanup
