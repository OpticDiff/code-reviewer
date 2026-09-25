# Test Coverage Rules

# no-wallclock-sleep-in-tests

## Focus
Unit tests must not rely on real wall-clock time.Sleep(...) for concurrency
synchronization or event arrival waiting, as this introduces severe CI pipeline
flakiness and non-deterministic test failures.
Synchronize concurrent routines using sync.WaitGroup, channels (<-done), or
errgroup. For Temporal workflows, use the test suite virtual clock (RegisterDelayedCallback).

## DO NOT Flag
Intentionally testing timeout triggers or rate limiter backoff
deadlines by sleeping longer than the configured timeout is permitted.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/testing/deterministic-tests

---

# test-coverage-meaningfulness

## Focus
Tests must perform meaningful assertions that verify behavior and state mutations,
rather than executing code solely to satisfy line coverage requirements ("coverage fraud").
Flag test patterns including:
- Invoking functions with blank identifier returns (_ = DoSomething()) and zero assertions
- Tautological assertions (e.g., assert.True(t, true), assert.NotNil(t, res) when res fields are empty)
- Modifying exported domain logic or critical business rules with zero corresponding test updates

## DO NOT Flag
`Benchmark*` tests (using `b.N`), `Fuzz*` tests (relying on crashes),
and `Example*` tests (relying on `Output` comments) are exempt from explicit assertions.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/testing/assertion-standards

---

# temporal-workflow-test-suite

## Focus
Temporal workflows located in internal/workflow/ must be tested using Temporal's
official testsuite package: go.temporal.io/sdk/testsuite.
Workflows must be executed in a test environment via env.ExecuteWorkflow(),
activity invocations must be mocked or registered via env.OnActivity(...),
and temporal events or delayed signals must utilize virtual time manipulation
(env.RegisterDelayedCallback or env.Now) rather than real system time.

## DO NOT Flag
Standard exclusions apply.

## Severity
Blocking

## Reference
https://docs.opticdiff.dev/architecture/temporal-testing

---

# test-error-path-coverage

## Focus
Changes introducing complex error-handling branches, retries, fallbacks, or compensating
logic should include corresponding negative unit test cases in *_test.go.
Verifying error paths prevents silent failures during network partitions, database
downtime, or malformed payload encounters.

## DO NOT Flag
Standard Go boilerplate error checks (if err != nil { return err })
without specialized domain recovery or custom error wrapping are exempt.

## Severity
Advisory

## Reference
https://docs.opticdiff.dev/testing/negative-testing

---

