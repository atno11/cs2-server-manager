# Testing and regression policy

Run `scripts/check.sh` from the repository root. It performs non-mutating formatting verification, vet, uncached tests, race tests, coverage with a temporary output file, build, and Git whitespace validation when a real Git repository is available. It prints PASS, FAIL, or BLOCKED for every check and exits nonzero for failure or blockage. Temporary Go cache and build directories are created under `${TMPDIR:-/tmp}`; no persistent Go configuration is changed. A build without VCS stamping is additionally attempted when Git is blocked, but it does not turn a blocked standard build into PASS.

For a bug fix: reproduce; diagnose the root cause; add a regression test; confirm it fails before the fix when feasible; make the smallest correct change; run the targeted test; run the full accumulated suite; record evidence and update documentation. Explain any infeasible pre-fix reproduction. Never remove or weaken tests to pass. Coverage is diagnostic, not a percentage gate. Favor meaningful behavior, concurrency, invalid inputs, and failure paths. Use isolated fake processes for unit tests, bounded asynchronous waits, no arbitrary sleeps, and no ordering dependencies. Report native CS2 validation separately; it remains pending without a verified installation. Do not put credentials in fixtures, output, or documentation.

Initial Phase 0.1 baseline and final results are in `phases/phase-00-1.md`.

Phase 1.1 adds isolated shell-script server tests for concurrency, process groups, forced termination, timeout reporting, HTTP shutdown ordering, and resource bounds. The repeated targeted race command and results are in `phases/phase-01-1.md`. Tests use bounded polling for readiness; no native CS2 process is launched.

Phase 1.1.1 uses Linux pidfd and `/proc` with controlled fake processes. The phase report records an intermittent full race-test failure and later passing repetitions; do not silently discard future recurrence. HTTP shutdown failure tests use in-memory pipe connections and a short injected deadline, without opening a host port.

Phase 1.1.2 established FLAKY-001 as transient `/proc` `ESRCH` during member scanning. The regression injects that exact error and confirms a later live group member is still found. The phase report retains all pre-fix failures and post-fix repetition counts; future recurrence should be investigated with the test's printed `Instance.Error`.

Phase 1.2 adds isolated HTTP, trust-policy, credential, redaction, permission and capability tests. Test fixtures explicitly authorize their fake executables. The phase report records the pre-change 29-test baseline, targeted/repeated race checks, final complete validation and coverage. `GOCACHE` and `GOTMPDIR` may be set to private temporary directories for direct Go commands when the sandbox's default Go cache is read-only.

Phase 1.2.1 adds regression cases for overlapping secret intervals, changed trusted roots/executable metadata, persisted size/count, ambiguous JSON keys, public secret exposure, secret-count boundaries, and descriptor execution with a copied Go test executable. The controlled executable test validates Linux behavior in the test environment, not native CS2. The phase report records pre-fix failures, repeated race checks, and final results.


The Phase 1.2.1 HTTP follow-up adds regression tests for schema stability with a known secret matching a JSON key, successful output above 64 KiB, and empty error bodies with preserved status codes and authentication challenges. Log bounds and split-write tests remain in the complete suite.

Phase 1.3 uses temporary real SQLite files for schema, migration, corruption, permission, lock, busy-timeout and transaction tests. Manager failure tests inject a narrow persistence failure while retaining real database coverage elsewhere. Historical JSON-input tests now run against a fresh migration directory; the original input-validation assertions remain. Tests that reopen the manager first close the original owner because one manager now exclusively owns a runtime directory. The full phase report records adapted tests, dependency reproducibility and native CS2 limits.
