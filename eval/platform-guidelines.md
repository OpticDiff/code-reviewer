# Platform Security Guidelines

## Mandatory Requirements

All code changes MUST comply with the following security requirements:

1. **No SQL Injection**: All database queries must use parameterized statements
2. **No Hardcoded Secrets**: API keys, passwords, tokens must come from env vars or secret managers
3. **Authorization Required**: All HTTP handlers must verify caller authorization
4. **Secure TLS**: TLS 1.2+ minimum, no InsecureSkipVerify
5. **No Path Traversal**: File paths must be sanitized and validated
6. **No XSS**: Use html/template for HTML rendering
7. **Data Race Prevention**: Shared state must use proper synchronization
