# Phase 1.2.1 — Security review fixes

Objective: investigate the independent review and correct confirmed security and compatibility defects without adding product features. The fixed checklist was recorded before application changes; all criteria begin PENDING and their wording will remain unchanged.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| SCR-01 | Known secret redaction handles overlapping, adjacent, nested and split-write matches independently of secret configuration order. Bounded log behavior is preserved. | PASS | `TestReviewOverlappingKnownSecrets` and `TestReviewPendingBound`; 20-repeat targeted race pass. |
| SCR-02 | Trusted executable and working roots are revalidated before managed launches according to the documented filesystem threat model. | PASS | Root-mode mutation tests reject start and open; `secureWorkTree`/`secureExecTree` now check roots. |
| SCR-03 | Executable identity verification includes ownership, permissions, regular-file status and content identity. Unsafe metadata changes fail closed. | PASS | Mode-bit tests and capability getter seam; descriptor owner/mode/inode/digest checks. |
| SCR-04 | Persisted definitions enforce the documented instance count and input size limits during startup. Invalid persisted state fails explicitly without partial loading or destructive modification. | PASS | 33-definition and oversized state tests reject without rewriting JSON. |
| SCR-05 | Ambiguous JSON documents containing duplicate or conflicting recognized fields are rejected at the relevant input boundaries. | PASS | HTTP, policy and persisted duplicate-key tests, including case aliases. |
| SCR-06 | Known secrets cannot be exposed through accepted public instance fields or public error responses. Preserve stable instance identifiers. | PASS | Secret ID rejection and public-error response redaction tests. |
| SCR-07 | Operator-configured secret limits and internal credential redaction limits are consistent. | PASS | `TestReviewSecretLimitsAndPublicConstants`: 0/64 accepted, 65 rejected, internal token separate. |
| SCR-08 | Audit descriptor-based execution compatibility, argv[0], executable identity and resource resolution. Document verified behavior and unverified native CS2 compatibility separately. | PASS | `TestReviewDescriptorExecution` verified native Go fixture behavior; CS2 remains unverified. |
| SCR-09 | Deterministic regression tests cover confirmed security defects and rejection paths. | PASS | 15 new top-level tests; original 44 retained; pre-fix failures recorded. |
| SCR-10 | All applicable existing and new regression checks, race checks, builds and scripts/check.sh pass. Documentation contains criterion-level evidence. | PASS | Full race count=3 exit 0; `bash scripts/check.sh` exit 0; docs and coverage recorded. |

## Baseline

Branch `feat/security-hardening`, HEAD `a0e8b20`; working tree clean before this phase. Phase 1.2 reported 44 top-level tests, 76.9% overall coverage, and `scripts/check.sh` exit 0. Native CS2 validation remains pending. The local pre-change check is recorded below.

## Baseline evidence

Pre-change `bash scripts/check.sh` exited 0 on clean `feat/security-hardening` at `a0e8b20`: gofmt, vet, uncached tests, race, coverage, build and Git whitespace checks all passed. `go test -list '^Test' ./...` showed 44 top-level tests before changes. The local coverage run reported cmd 66.9%, manager 79.5%, security 74.4%, overall 77.0%, compared with the published 76.9% overall baseline. This small difference reflects the measured local run, not a changed acceptance target. Native CS2 tests were not run.

## Review findings and reproduction

| Finding | Outcome | Evidence before fix |
| --- | --- | --- |
| SR-R01 overlapping redaction | Confirmed | `TestReviewOverlappingKnownSecrets` leaked `fg` after redacting `abcde` from `abcdefg`; adjacent matches produced two separate markers. |
| SR-R02 root revalidation | Confirmed | `TestReviewRootAndExecutableMetadataRecheck` started after the trusted root was changed to mode 0777. |
| SR-R03 executable metadata | Confirmed by code inspection and test | `openExecutable` checked device/inode, regular status, execute bit and digest but omitted owner, group/world write, setuid/setgid and file-capability checks. The pre-fix combined root/metadata test stopped at its first root failure; separate metadata assertions ran after correction. |
| SR-R04 persisted limits | Confirmed | `TestReviewPersistedDefinitionCountAndDuplicateJSON` loaded 33 valid definitions despite the 32-instance registration limit. |
| SR-R05 ambiguous JSON | Confirmed | Pre-fix policy and HTTP duplicate-field tests accepted case-insensitive aliases; persisted duplicate validation was also absent. |
| SR-R06 public known secrets | Confirmed | `TestReviewKnownSecretInID` registered `secret-instance` while `secret` was configured for redaction. App-generated generic errors also lacked an output redaction boundary. |
| SR-R07 secret count | Confirmed by code inspection | `main` appended one internal token to up to 64 operator secrets before `compilePolicy` applied a 64-total cap. |
| SR-R08 descriptor execution | Compatibility concern, not a reproduced security defect | `exec.Command("/proc/self/fd/3", ...)` set `argv[0]` to the descriptor path; controlled native test behavior is documented below. No native CS2 installation was launched. |

The initial `go test -count=1 -run '^TestReview' ./internal/manager ./cmd/cs2m` exited 1 with overlapping, root, persisted count, public ID, policy JSON and HTTP JSON failures. No existing test assertion was weakened. A later targeted run exited 1 because a new test used the Unix octal literal `04700` as a Go `os.FileMode` instead of `os.ModeSetuid`; this was a test-fixture error. The test was corrected to use Go mode bits, then targeted runs passed. This failure did not indicate a production defect.

## Corrections

The ring now tracks the union of secret match intervals and carries coverage across writes. Its pending input stays bounded by the longest secret and the 4096-byte write chunk; output remains bounded at 64 KiB. Roots are checked at every start; the open executable is rechecked for owner, mode, file capabilities, inode and digest. Persisted JSON reads are capped at 512 KiB, 32 definitions and 16 KiB per compact definition; invalid data returns an error without rewriting the file. HTTP, policy and persisted JSON reject duplicate keys and case-insensitive aliases, with a 64-level nesting cap. Known secrets in IDs/ports and fixed public status labels fail validation, while app-generated API responses are redacted. Operator secret count is capped at 64 independently of the one internal token.

`exec` still uses the verified descriptor, with the authorized original path in `argv[0]`. A copied Go test executable confirmed `argv[0]`, `/proc/self/exe`, FD 3, per-instance working directory and a resource adjacent to `argv[0]`. A native CS2 executable may resolve resources differently; its compatibility remains unverified. Scripts may receive an interpreter-specific path rather than the supplied `argv[0]`.

## Remaining limits

The same-UID and trusted-parent-directory assumptions from Phase 1.2 remain. A change after verification and before execution under a hostile same-UID process is not ruled out. File capability presence and inspection-error branches were tested through a controlled getter seam; the tests did not modify real privileged file capabilities. Unknown secrets cannot be recognized. Native CS2 compatibility and validation are PENDING.

## Validation and Git state

Targeted post-fix `go test -count=1 -run '^TestReview' ./internal/manager ./cmd/cs2m` exited 0. Repeated `go test -race -count=20 -timeout=180s -run '^(TestReviewOverlappingKnownSecrets|TestReviewPendingBound|TestSimultaneousIndependentAndConcurrentStarts|TestStartStopAndShutdownRaces)$' ./internal/manager` exited 0. Final complete validation and coverage follow.

Branch `feat/security-hardening`, HEAD `a0e8b20`; Codex's `.git` mount is read-only. No stage, commit, push, merge or deployment was performed in this phase.

Final `GOCACHE=/tmp/cs2m-scr-cache GOTMPDIR=/tmp go test -race -count=3 -timeout=180s ./...` exited 0 across all three packages. Final `bash scripts/check.sh` exited 0: gofmt, vet, uncached tests, race, coverage generation, standard build and Git diff check all PASS. The suite contains 59 top-level tests (44 prior plus 15 new). Coverage: cmd 69.4%, manager 80.8%, security 74.4%, overall 78.6%, versus local pre-change 77.0% and published 76.9%. Coverage is diagnostic, not a gate. Native CS2 validation remains PENDING.
