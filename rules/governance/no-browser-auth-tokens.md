# no-browser-auth-tokens

## Focus
Authentication tokens, JWTs, Macaroons, and API keys must NEVER be stored in JavaScript-accessible browser storage
(localStorage, sessionStorage, or JavaScript memory variables). Secure, HTTP-only cookies managed by the platform shell for session state are permitted.
In AzraONE, the BFF (Backend-For-Frontend) holds all session state and upstream credentials.
The browser communicates strictly via secure HTTP-only cookies managed by the platform shell.
Direct token management, custom login forms, or storing tokens in browser storage violates
SOC 2 CC6.1 and HITRUST 01.b.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/compliance/no-browser-auth-tokens
