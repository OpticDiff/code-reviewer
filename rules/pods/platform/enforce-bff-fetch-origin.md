# enforce-bff-fetch-origin

## Focus
Frontend product modules and Module Federation remotes must route all API calls
strictly through the platform BFF using shell.bffFetch() provided by @azra/module-contract,
or through typed Connect-Web client stubs pointing to shell-bff.
Direct calls using raw window.fetch(), axios(), or targeting internal microservice
hostnames directly are prohibited. The BFF provides authentication, tenant validation,
and anti-spoofing header injection.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/bff-gateway
