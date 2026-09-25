# hipaa-phi

## Focus
Detect unmasked Protected Health Information (PHI) in code changes.
All PHI fields (SSN, MRN, patient name, DOB, email, phone, address,
insurance IDs) must be masked using phi.Mask() from platform-sdk-go
before logging, transmission, or storage in non-encrypted fields.
Direct string interpolation of PHI into log statements, error messages,
or API responses is a critical violation.

## DO NOT Flag
Only exclude `**/mocks/**`. Do not exclude test files.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/hipaa-phi-handling
