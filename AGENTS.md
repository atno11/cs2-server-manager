# Engineering instructions

For every implementation phase:

1. Read relevant project documentation.
2. Inspect the Git branch and working tree.
3. Preserve existing user changes.
4. Define the objective and explicit scope.
5. Record a fixed acceptance checklist with unique IDs before implementation.
6. Establish the relevant pre-change test baseline.
7. Implement changes incrementally.
8. Add tests for new behavior.
9. Add regression tests for corrected defects.
10. Run targeted tests during development.
11. Run the complete applicable regression suite before phase closure.
12. Record evidence for every acceptance criterion.
13. Update project documentation.
14. Report Git status and remaining blockers.
15. Provide a concise transition prompt when needed.

Never silently expand scope, weaken or remove tests to obtain passing results, report unexecuted tests as PASS, fabricate counts or evidence, or conceal failures. Preserve established behavior. Prefer deterministic, isolated tests; avoid arbitrary sleeps. Bound asynchronous waits. Test concurrent lifecycle operations, failure paths, and invalid inputs. Keep tests independent of execution order and real CS2 installations. Report simulated and native CS2 validation separately. Never expose credentials in tests, logs, or documentation. Never push, merge, or deploy without authorization.

A phase with unresolved mandatory criteria is not fully accepted. Report implementation completion separately from functional acceptance. Run `scripts/check.sh` and record its exit status, including blocked checks. For a bug fix, reproduce, diagnose, add a regression test, verify pre-fix failure when feasible, apply the smallest correct fix, verify the targeted test, run the accumulated suite, and document evidence. If pre-fix reproduction is infeasible, explain why. Coverage is diagnostic, with no arbitrary pass percentage. See `docs/TESTING.md`.
