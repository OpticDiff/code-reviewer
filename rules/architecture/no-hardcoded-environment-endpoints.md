# no-hardcoded-environment-endpoints

## Focus
Application code, CLI commands, and worker runtimes must not hardcode
environment-specific domains (e.g. *.dev.azra-ai.com, *.staging.azra-ai.com,
or localhost:*) in source code.

Canonical Configuration Hierarchy:
1. Explicit CLI flag (e.g. --endpoint, --webhook-url)
2. Environment variable (e.g. AZRA_WEBHOOK_URL, AZRA_ENDPOINT)
3. Configuration file (e.g. ~/.azra/config.yaml)
4. Platform production default fallback or discovery service

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/configuration
