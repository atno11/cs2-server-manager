# Phase 0.1: project foundation and regression baseline

The following fixed acceptance checklist was recorded before repository modifications. Status and evidence appear below; the criteria are unchanged.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| P01-01 | Audit existing source files, tests, dependencies, architecture and implemented behavior. | PASS | Source inventory in `../ARCHITECTURE.md` and `../PROJECT_STATUS.md`. |
| P01-02 | Preserve existing user files and functionality. No unrelated modifications or destructive operations. | PASS | Only documentation and `scripts/check.sh` changed in this phase; application source and existing tests remain untouched. |
| P01-03 | Diagnose Git restrictions and establish or verify a usable Git repository when safely permitted. | PASS | Host-side `git init -b main` succeeded; Codex-side `git status` confirms `main` with no commits. `.git` remains read-only to Codex. |
| P01-04 | Verify a functional Go build and test environment. | PASS | Go 1.27.3; temporary `GOCACHE` and `GOTMPDIR` permit builds and tests. |
| P01-05 | Execute and document the existing test baseline before changing application behavior. | PASS | Three tests passed before documentation edits; commands and results below. |
| P01-06 | Create or reconcile permanent project documentation. | PASS | README, architecture, roadmap, testing, status, phase reports; historical `AUDIT.md` retained. |
| P01-07 | Create or update AGENTS.md with permanent engineering, testing and regression requirements. | PASS | Root `AGENTS.md` contains phase workflow and testing rules. |
| P01-08 | Provide a reproducible automated validation command. | PASS | `scripts/check.sh` runs required checks; host-side execution exited 0. |
| P01-09 | Execute final baseline validation and document results. | PASS | Host-side `scripts/check.sh` exited 0 with all checks passing; earlier Codex-side run and limits are retained below. |
| P01-10 | Record project status, blockers, Git state and the recommended next phase. | PASS | See `../PROJECT_STATUS.md` and final report. |

## Initial baseline, before repository edits

Environment: `GOCACHE=/tmp/cs2m-p01-cache GOTMPDIR=/tmp/cs2m-p01-tmp`. `go test -count=1 -v ./...` exited 0: three tests passed (`TestAPI`, `TestLifecyclePersistenceAndValidation`, `TestUnexpectedExitAndLogs`). `go vet ./...` exited 0. `gofmt -l cmd internal` exited 0 and printed no files. Plain `go build ./...` failed because VCS stamping could not read a valid repository (Git exit 128). `go build -buildvcs=false ./...` exited 0. `go test -race -count=1 ./...` exited 0 in both packages. `go test -coverprofile=/tmp/cs2m-p01-coverage.out -count=1 ./...` exited 0: 42.4% in `cmd/cs2m`, 71.8% in `internal/manager`, 63.4% total statements. These percentages are diagnostic only. Native CS2 testing was not run.

## Final baseline

`scripts/check.sh` exited 1. PASS: gofmt, vet, three uncached tests, race tests, coverage generation, and `go build -buildvcs=false ./...`. Coverage on the final run: 42.4% in `cmd/cs2m`, 72.3% in `internal/manager`, 63.8% total. BLOCKED: standard `go build ./...` and `git diff --check`, because `.git` is not a usable repository. No Go check failed. The small coverage variation from the initial run reflects execution path variation in existing tests, not a code change. Native CS2 tests were not run. At the time of this Codex-side run, Phase 0.1 was not fully accepted because P01-03 was pending and the validation entry point exited nonzero. The later host-side closure below supersedes that status.

## Closure after host-side Git repair

**Host-side evidence supplied by the user:** `git init -b main` passed with Git 2.56.0; the repository has no commits and existing files were preserved. `scripts/check.sh` exited 0. Formatting, vet, automated tests, race detector, coverage generation, standard Go build, and Git diff check all passed. Official host-side coverage baseline: 42.4% `cmd/cs2m`, 71.8% `internal/manager`, 63.4% overall. This supersedes the earlier Codex-side 63.8% run as the official baseline; both historical results remain recorded above. The host-side report did not provide a separate test count, so the verified count remains the three existing tests observed in the Codex-side baseline.

**Codex-side verification at closure:** `git status --short --branch` reports `main`, no commits, and all intended files untracked. `.git` is readable but mounted read-only (`ext4`, `ro,nosuid,nodev,noatime`) in the Codex sandbox; Codex cannot stage or commit here. The intended-file audit found no matches for common credential patterns and no generated binaries, logs, database files, or coverage files. `.gitignore` excludes expected build, coverage, runtime, local config, and editor artifacts. This is a pattern audit, not a guarantee that arbitrary text contains no secrets. `git diff --check` exits 0 on the untracked tree, but has no content to inspect until files are staged; host-side validation reported it passing. No files were staged or committed.

**Acceptance:** all fixed P01 criteria are PASS using the combined, clearly attributed host-side and Codex-side evidence. Phase 0.1 is closed. Native CS2 validation remains PENDING because no verified installation is available; it is outside this phase's acceptance checklist.
