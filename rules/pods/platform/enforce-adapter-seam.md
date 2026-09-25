# enforce-adapter-seam

## Focus
Entrypoint and CLI command packages (cmd/**) must remain thin orchestration
layers. Do not instantiate raw HTTP clients, assemble protocol headers,
read unredacted auth tokens, or execute forge REST calls directly in cmd/.
Delegate external forge and system interactions to internal adapter packages
behind mockable interfaces.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/adapter-seams
