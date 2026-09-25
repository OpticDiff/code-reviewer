# zero-trust-secrets

## Focus
All secrets and credentials must be consumed via External Secrets
Operator (ESO) syncing from the platform secret store. Flag violations including:
- Hardcoded API keys, tokens, passwords, or connection strings
- Direct cloud credential instantiation (e.g., google.DefaultClient
  outside of approved platform-sdk-go wrappers)
- Plain text secrets in environment variable defaults
- Unencrypted internal service communication (must use ConnectRPC mTLS)
- Secrets committed in config files, even if "for testing"

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/security/secrets-management
