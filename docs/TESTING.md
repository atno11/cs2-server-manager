# Testing and regression policy

Run `scripts/check.sh` from the repository root. It performs non-mutating formatting verification, vet, uncached tests, race tests, coverage with a temporary output file, build, and Git whitespace validation when a real Git repository is available. It prints PASS, FAIL, or BLOCKED for every check and exits nonzero for failure or blockage. Temporary Go cache and build directories are created under `${TMPDIR:-/tmp}`; no persistent Go configuration is changed. A build without VCS stamping is additionally attempted when Git is blocked, but it does not turn a blocked standard build into PASS.

For a bug fix: reproduce; diagnose the root cause; add a regression test; confirm it fails before the fix when feasible; make the smallest correct change; run the targeted test; run the full accumulated suite; record evidence and update documentation. Explain any infeasible pre-fix reproduction. Never remove or weaken tests to pass. Coverage is diagnostic, not a percentage gate. Favor meaningful behavior, concurrency, invalid inputs, and failure paths. Use isolated fake processes for unit tests, bounded asynchronous waits, no arbitrary sleeps, and no ordering dependencies. Report native CS2 validation separately; it remains pending without a verified installation. Do not put credentials in fixtures, output, or documentation.

Initial Phase 0.1 baseline and final results are in `phases/phase-00-1.md`.

Phase 1.1 adds isolated shell-script server tests for concurrency, process groups, forced termination, timeout reporting, HTTP shutdown ordering, and resource bounds. The repeated targeted race command and results are in `phases/phase-01-1.md`. Tests use bounded polling for readiness; no native CS2 process is launched.

Phase 1.1.1 uses Linux pidfd and `/proc` with controlled fake processes. The phase report records an intermittent full race-test failure and later passing repetitions; do not silently discard future recurrence. HTTP shutdown failure tests use in-memory pipe connections and a short injected deadline, without opening a host port.
