# Phase 1.1.2 — Stability validation

Fixed checklist recorded before modifications. All criteria begin PENDING. No criterion text will change.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| STB-01 | Record Git state, baseline results, kernel version, Go version and relevant environment characteristics. | PASS | Clean branch `feat/process-lifecycle-hardening` at `8d32143`; baseline `bash scripts/check.sh` exit 0, 28 tests, 76.5% overall; Go and kernel inventory below. |
| STB-02 | Audit FLAKY-001, its fixture, synchronization, assertions, process ownership and cleanup paths. | PASS | FLAKY-001 reproduced as transient `/proc/<pid>/stat` `ESRCH`; fixture and cleanup-path audit below. |
| STB-03 | Complete 100 isolated repetitions of the affected test without unexpected failures. | PASS | Post-fix JSON run: 100 runs, 100 passes, 0 failures, exit 0. |
| STB-04 | Complete 50 isolated race-detector repetitions of the affected test without unexpected failures. | PASS | Post-fix race JSON run: 50 runs, 50 passes, 0 failures, exit 0. |
| STB-05 | Verify that repeated tests do not leave accumulating live owned processes or persistent resource leaks. | PASS | Each affected iteration asserts its child is not live; 20 FD-cycle test repetitions (240 cycles) passed; no live `server.sh` remained in process inventory. |
| STB-06 | Verify the required pidfd group-signaling capability and readable /proc behavior in the test environment. | PASS | Kernel 7.2; `/proc/self/stat` and status readable; header defines group flag; descendant TERM test and pidfd-backed launch passed. |
| STB-07 | Complete the applicable regression suite and scripts/check.sh successfully. | PASS | `go test -race -count=3 -timeout=180s ./...` and final `bash scripts/check.sh` exited 0. |
| STB-08 | Document exact evidence, limitations, failures, Git state and the merge-readiness recommendation. | PASS | This report records pre/post-fix failures, limits, coverage, Git state and recommendation. |

## Baseline and environment

Baseline `bash scripts/check.sh` exited 0: formatting, vet, 28 top-level tests, race, coverage, standard build, and Git diff check passed. Coverage: 59.0% application, 80.7% manager, 76.5% overall. Initial inspection: clean worktree on `feat/process-lifecycle-hardening`, HEAD `8d32143ac7220206ceddc63f336d214f27308e4c`, tracking origin. Go `go1.27.3-X:nodwarf5 linux/amd64`; Arch Linux kernel `7.2.11-arch1-1`; user `atno` UID 1000. Workspace ext4 is writable; `.git` remains read-only to Codex; `/proc` is mounted read-only inside the sandbox, which permits reads. No native CS2 installation is verified.

## FLAKY-001

An earlier Phase 1.1.1 full race run failed once in `TestUnexpectedLeaderExitCleansDescendant`; the original failure detail was not captured. Subsequent full and isolated runs passed. Root cause remains unconfirmed. This phase investigates it without assuming an application or fixture defect.

## Investigation and confirmed cause

The fake fixture runs `sleep 30` as an owned descendant, records its PID, and exits the leader. The test waits for a terminal manager state, reports `Instance.Error` on failure, then verifies the recorded descendant is not live. Its assertions were retained. The supervisor uses `waitid(WNOWAIT)` to retain the leader identity, pidfd group signaling for TERM, and `/proc` group-member scans for completion. A member can exit between `os.ReadDir("/proc")` and `os.ReadFile("/proc/<pid>/stat")`. The scanner already skipped `ENOENT`, but a disappearing proc stat returned `ESRCH`, which Go does not classify as `os.ErrNotExist`. The manager treated that expected disappearance as a group-cleanup failure and set the instance to `failed`.

The first isolated 100-run JSON batch produced 97 passes and 3 failures. Every failure reported `process-group cleanup: read /proc/<pid>/stat: no such process`, with distinct transient PIDs. The command wrapper used zsh's reserved `status` variable after the Go invocation, so it did not print the Go exit code; the JSON package final action was `fail`, and the wrapper exited 1. A pre-fix race batch ran 50/50 passes (exit 0), demonstrating that the defect is intermittent. Failure output was preserved in `/tmp/cs2m-stb-normal.json` during this session.

A small test seam extracted the existing scan loop without changing its behavior. `TestDisappearingProcEntryIsIgnored` injected a wrapped `syscall.ESRCH` for one entry and a live owned member for another. It failed before the fix and passed after the scanner treated `ESRCH` like `ENOENT`. Other read errors still fail closed. No production behavior outside transient `/proc` disappearance handling changed.

## Post-fix validation and resources

All explicit `go test` commands in this phase used `GOCACHE=/tmp/cs2m-stability-cache GOTMPDIR=/tmp`; `scripts/check.sh` created its own temporary Go cache and build directory. JSON diagnostics were stored under `/tmp` and were not added to the repository.

`go test -json -count=100 -timeout=15m -run '^TestUnexpectedLeaderExitCleansDescendant$' ./internal/manager`: 100 runs, 100 passes, 0 failures, exit 0. `go test -race -json -count=50 -timeout=15m -run '^TestUnexpectedLeaderExitCleansDescendant$' ./internal/manager`: 50 runs, 50 passes, 0 failures, exit 0. JSON run events confirmed the target test actually executed. `go test -count=20 -timeout=5m -run '^TestRepeatedCyclesDoNotAccumulateFDs$' ./internal/manager` exited 0; the test performs 12 process cycles per repetition and checks its file-descriptor bound. Every passing affected-test iteration also verifies its recorded descendant is not live. A post-run `ps` inventory found no live `server.sh` process. This is concrete evidence for these fixtures, not a host-wide leak guarantee.

`/proc/self/stat` and `/proc/self/status` were readable; `/usr/include/linux/pidfd.h` defines `PIDFD_SIGNAL_PROCESS_GROUP` as bit 2. Kernel `7.2.11-arch1-1` exceeds the required Linux 6.9. `TestNaturalLeaderExitGivesDescendantGrace` exited 0 and exercises pidfd-backed group TERM and `/proc` membership checks. Capability is verified for this sandbox, not for other hosts.

`go test -race -count=3 -timeout=180s ./...` exited 0. Final `bash scripts/check.sh` exited 0: gofmt, vet, full tests, race, coverage, standard build and Git diff check passed. There are 29 top-level tests, including one new deterministic regression. Final statement coverage: 59.0% application, 80.7% manager, 76.6% overall; baseline was 76.5%. Coverage is diagnostic.

## Status and recommendation

FLAKY-001 is a **confirmed application defect with a targeted fix** under the tested conditions. The original failure signature was reproduced, and the deterministic regression failed before and passed after the fix. The 100/100 and 50/50 post-fix batches support stability for this fixture; they do not prove the absence of every future race or host-specific `/proc` behavior. Detached descendants, abrupt manager termination, older kernels, and restricted `/proc` visibility remain documented limits. Native CS2 validation remains PENDING.

Branch `feat/process-lifecycle-hardening` at HEAD `8d32143ac7220206ceddc63f336d214f27308e4c` before this phase. The Codex `.git` mount is read-only; current changes are unstaged and uncommitted. Based on the completed checklist, this targeted issue no longer blocks merge review after the fix is committed and independently checked. Do not merge without the normal review. No push, merge or deployment occurred in this phase.
