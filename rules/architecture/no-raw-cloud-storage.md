# no-raw-cloud-storage

## Focus
Service and worker code must never directly import raw cloud storage SDKs or
bypass the platform vendor-neutral storage seam. All blob and object storage
operations must go through approved platform storage interfaces (e.g., temporal/storage
or platform-sdk-go), with drivers and endpoints bound at runtime via configuration.

Flag violations including:
- Direct cloud SDK imports: cloud.google.com/go/storage, github.com/aws/aws-sdk-go-v2/service/s3,
  or azure-sdk storage packages.
- AI workarounds and CLI exec: Spawning subprocesses via os/exec (or equivalent) to execute
  cloud CLIs (gcloud, aws, gsutil, az).
- Metadata scraping: Direct queries to link-local instance metadata services (e.g., 169.254.169.254)
  to discover credentials, tokens, or cloud identity.
- Hardcoded cloud URLs: Embedding provider-specific URL schemes (gs://, s3://) or cloud endpoints
  (e.g., storage.googleapis.com) in code, templates, or migrations instead of protocol-agnostic identifiers.
- Unapproved raw REST clients: Crafting direct HTTP/REST calls against cloud provider storage
  endpoints without using the approved platform seam.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/storage-seam
