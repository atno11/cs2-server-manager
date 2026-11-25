# Phase 1.2 — Security hardening

The fixed checklist below was recorded before application changes. All statuses begin PENDING. Criteria will remain unchanged.

| ID | Criterion | Status | Evidence |
| --- | --- | --- | --- |
| SEC-01 | Document the threat model, trusted operator boundaries, untrusted HTTP inputs, filesystem assumptions and explicitly unsupported security scenarios. | PASS | `docs/SECURITY.md` threat and same-UID boundary. |
| SEC-02 | All administrative routes require authentication. The browser UI and programmatic API remain usable through documented authentication mechanisms. | PASS | `TestAuthenticationHostOriginAndMethods`; token bootstrap test; all `/api/` routes gated. |
| SEC-03 | Unauthorized cross-origin state-changing browser requests are rejected, including requests that do not require a CORS preflight. | PASS | `TestAuthenticationHostOriginAndMethods` rejects foreign Origin and Fetch-Site, including simple POST. |
| SEC-04 | Unexpected Host headers and DNS rebinding attempts are rejected. Only explicitly trusted local origins and authorities are accepted. | PASS | `TestAuthenticationHostOriginAndMethods` rejects wrong Host/rebinding authority; forwarded headers ignored. |
| SEC-05 | HTTP routes enforce their exact allowed methods. Invalid method and path combinations cannot mutate instance state. | PASS | `TestAuthenticationHostOriginAndMethods` verifies methods and path/action combinations. |
| SEC-06 | JSON inputs reject malformed documents, unknown fields, trailing JSON values and oversized bodies. Invalid input cannot partially mutate state. | PASS | `TestStrictJSONAndNoPartialMutation` covers malformed, unknown, trailing, large bodies. |
| SEC-07 | HTTP request handling and configurable instance registration have documented resource limits. Slow or oversized requests cannot consume unbounded application resources. | PASS | `TestConnectionLimit`, HTTP deadlines and documented 64/16 KiB/32-instance limits. |
| SEC-08 | HTTP callers cannot select arbitrary executables. Execution is restricted to explicitly trusted operator-configured executable identities. | PASS | `TestExecutionTrustAndPathIdentity` and `TestHTTPUntrustedExecutableRejectedWithoutPathLeak`. |
| SEC-09 | Working directories and executable paths are validated against trusted roots. Equivalent paths cannot bypass instance isolation or duplicate-directory validation. | PASS | `TestExecutionTrustAndPathIdentity` covers roots, symlink and duplicate identity. |
| SEC-10 | Symlink and path-replacement risks are addressed according to the documented filesystem threat model. Do not claim TOCTOU resistance without evidence. | PASS | `TestExecutionTrustAndPathIdentity` covers replacement; descriptor launch and same-UID limit documented. |
| SEC-11 | Launch arguments are structurally validated. Managed ports and protected launch settings cannot be overridden through accepted argument forms. | PASS | `TestArgumentAndInstanceLimits` rejects protected option forms and oversized input. |
| SEC-12 | Secrets are not returned through instance API responses, HTML, registration responses or errors. Authentication credentials are never logged. | PASS | `TestAuthenticationHostOriginAndMethods` and token tests; API projects public fields only. |
| SEC-13 | Known secret values are redacted from captured logs, including values split across writes where applicable. Document why arbitrary unknown secrets cannot be guaranteed to be detected. | PASS | `TestKnownSecretRedactionAcrossWrites` verifies split writes and bound. |
| SEC-14 | Sensitive runtime directories and persisted files have verified restrictive ownership and permissions. Unsafe existing paths fail explicitly without silently modifying user permissions. | PASS | `TestRuntimeAndPersistedPermissions`, `TestTokenBootstrapAndPermissions`, `TestSymlinkEscapeAndPolicyFilePermissions`. |
| SEC-15 | Required Linux process-supervision capabilities are verified before any managed executable is launched. Unsupported environments fail safely. | PASS | `TestCapabilityRejectionBeforeManagerCreation`; controlled pidfd and /proc probe before starts. |
| SEC-16 | Deterministic security and regression tests verify the implemented trust boundaries and rejection paths. | PASS | 44 top-level tests, including 29 pre-existing; targeted security tests pass. |
| SEC-17 | All applicable existing and new tests, race checks, build checks and scripts/check.sh pass. | PASS | `go test -race -count=3 -timeout=180s ./...` exit 0; `bash scripts/check.sh` exit 0. |
| SEC-18 | Documentation records security decisions, limitations, criterion-level evidence, coverage and Git state. | PASS | This report, SECURITY, README, status, architecture, roadmap, testing updated. |

## Pre-change baseline

Clean worktree on `feat/security-hardening`, HEAD `7f68f77` (merge of PR #1). `bash scripts/check.sh` exited 0: formatting, vet, 29 top-level tests, race, coverage generation, standard build, and Git diff check passed. Coverage: `cmd/cs2m` 59.0%, `internal/manager` 80.7%, overall 76.6%. Native CS2 validation was not run.

## Verified findings and implementation

Pre-change inspection confirmed unauthenticated API and UI, generic instance routing, arbitrary executable registration, weak path validation, unredacted logs, and pidfd capability checks after launch. These were static findings; no pre-fix exploit test was run. The existing 29 tests were retained. Test fixtures gained explicit trusted policy and bearer credentials. No native CS2 process was used.

The operator now controls a 0600 `policy.json` with exact executables and allowed roots. The manager pins executable identity/hash, validates ownership and path components, and launches from an open descriptor. HTTP uses an in-memory browser bearer token, strict Host/Origin and Fetch Metadata checks, exact methods, bounded strict JSON and public response models. A controlled Linux capability probe runs before any managed executable launch. Known secret values are redacted across log writes. See `../SECURITY.md` for the threat model, exact limits and residual risks.

## Validation

- Pre-change: `bash scripts/check.sh` exit 0, 29 top-level tests, 76.6% overall (cmd 59.0%, manager 80.7%).
- During development: `GOCACHE=/tmp/cs2m-sec-cache GOTMPDIR=/tmp go test -count=1 -timeout=120s ./...` exit 0. An earlier direct `go vet ./...` without temporary cache failed because the sandbox default Go cache was read-only; this was environmental, and the required script used a writable temporary cache.
- Repeated race: `GOCACHE=/tmp/cs2m-sec-cache GOTMPDIR=/tmp go test -race -count=3 -timeout=180s ./...` exit 0 for all packages.
- Final: `bash scripts/check.sh` exit 0: gofmt, vet, uncached tests, race, coverage generation, build and `git diff --check` all PASS. 44 top-level tests; cmd 66.9%, manager 79.4%, internal/security 74.4%, overall 76.9%. The 0.3-point overall increase is diagnostic only.

## Limits and Git state

A malicious same-UID process is outside the filesystem trust boundary. Path checks and executable descriptors reduce but cannot eliminate same-UID replacement races; arbitrary unknown secrets are not detectable. Operator-controlled game files and indirect CS2 configuration semantics remain trusted. Detached process descendants and abrupt manager death remain lifecycle limits. Native CS2 validation is PENDING because no verified installation was used.

Branch `feat/security-hardening`, baseline HEAD `7f68f77`. Codex can read `.git` but cannot write it in this sandbox. Changes remain unstaged; no commit, push, merge or deployment was performed. Host-side review and commit are the next repository actions.
