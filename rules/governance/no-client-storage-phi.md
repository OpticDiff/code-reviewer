# no-client-storage-phi

## Focus
Protected Health Information (PHI) — including patient names, SSNs, MRNs, dates of birth,
diagnostic notes, and prescription data — must NEVER be stored or cached in localStorage,
sessionStorage, or unencrypted browser databases.
Client-side persistent storage is accessible to any script executing in the browser origin,
creating severe HIPAA, SOC 2, and HITRUST data leakage risks.
Keep clinical state ephemeral in component memory (React state / TanStack Query) only.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/no-client-storage-phi
