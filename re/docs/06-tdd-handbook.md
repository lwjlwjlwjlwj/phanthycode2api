# 06 · Test-Driven Development (TDD) Handbook

**Milestone:** M3 · Author: TDD sub-agent · Language: English
Date: 2026-10-26
Companion document: `05-sdd-spec.md` (requirement IDs `FR-01…FR-17`, `NFR-01…NFR-07`).

This handbook is normative for how tests are written, run, reviewed and gated in `phanthycode2api`.
Every baseline figure below was produced by actually running the commands in `/workspace` on 2026-10-26;
nothing here is estimated or invented.

---

## 1. TDD philosophy and the red-green-refactor loop

### 1.1 Why TDD here specifically

This repository is a **protocol translator**. Its risk is not "the function returns the wrong int"; its risk is
"the wire format silently degrades". Three properties make TDD load-bearing:

1. **Translation is total-function shaped.** `PrepareBody`, `Aggregate`, `Stream` and `Classify` each accept a large
   input space (every possible OpenAI request, every possible Anthropic SSE frame) and must never panic or emit
   malformed output. A test per behaviour is the only practical specification.
2. **The upstream cannot be reached in tests.** No test may make a network call to `https://code.phanthy.com`
   (`00-governance.md:18-20` caps authenticated calls at 5 total and forbids load). Therefore the *contract with the
   upstream* can only be encoded as **fixtures**, and fixtures can only be introduced test-first.
3. **Error classification drives money.** `Classify` decides whether an account is cooled for 60 s or 12 h or
   disabled forever. A misclassified marker silently destroys an account pool. Every marker must arrive with a test.

### 1.2 The loop

```
        ┌─────────────────────────────────────────────────────────────┐
        │  RED      write a test that expresses one new behaviour     │
        │           run it — it MUST fail, and fail for the right      │
        │           reason (assertion, not compile error / panic)      │
        └───────────────────────────────┬─────────────────────────────┘
                                        ▼
        ┌─────────────────────────────────────────────────────────────┐
        │  GREEN    write the smallest production change that makes    │
        │           that test pass; no extra fields, no extra branches │
        └───────────────────────────────┬─────────────────────────────┘
                                        ▼
        ┌─────────────────────────────────────────────────────────────┐
        │  REFACTOR make it clean (names, duplication, structure)      │
        │           while all tests stay green                         │
        └───────────────────────────────┬─────────────────────────────┘
                                        ▼
        ┌─────────────────────────────────────────────────────────────┐
        │  VERIFY   go test ./... ; go test -race ./... ; go vet ./... │
        └─────────────────────────────────────────────────────────────┘
```

Rules that make the loop real rather than ceremonial:

- A test that passes before the production change is **not** a TDD test — it is a characterisation test. Label it as
  such in the commit message (see §8.1) and do not pretend it drove design.
- A test that fails to *compile* is yellow, not red. Fix the signature first, then observe a red assertion.
- One behaviour per loop iteration. "Add new marker and refactor the marker list" is two iterations.
- No production code may be added because "a later test will need it".

### 1.3 Worked example — adding a new upstream error marker

**Scenario.** A user reports that their account was cooled for only 10 minutes after an upstream body saying
`"your credit limit has been reached"`. Today `Classify` returns `ErrClient` for that body because the string is not
in `hardMarkers` (`internal/upstream/client.go:61-67`). The desired behaviour: that body classifies as
`ErrHardCredit`, which cools the account for 12 h instead of 10 m.

This example is used because it is the smallest end-to-end change that exercises the real risk: a behaviour change in
the most dangerous function in the repo, driven entirely by a test.

#### Step 1 — RED: write the failing test

`internal/upstream/client_test.go` (file does **not** exist today — this loop creates it):

```go
package upstream

import (
	"net/http"
	"testing"
)

func TestClassify_HardCreditMarkers(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   ErrKind
	}{
		{"payment required status", http.StatusPaymentRequired, `{}`, ErrHardCredit},
		{"existing english marker", http.StatusOK, `{"error":{"message":"insufficient credit"}}`, ErrHardCredit},
		{"existing chinese marker", http.StatusBadRequest, `{"error":{"message":"积分不足"}}`, ErrHardCredit},
		{"credit limit reached", http.StatusForbidden, `{"error":{"message":"your credit limit has been reached"}}`, ErrHardCredit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(c.status, c.body); got != c.want {
				t.Errorf("Classify(%d, %q) = %s, want %s", c.status, c.body, got, c.want)
			}
		})
	}
}
```

Run it:

```bash
go test -run 'TestClassify_HardCreditMarkers' -v ./internal/upstream
```

Expected RED observation (this is what the loop must actually show):

```
=== RUN   TestClassify_HardCreditMarkers
=== RUN   TestClassify_HardCreditMarkers/payment_required_status
=== RUN   TestClassify_HardCreditMarkers/existing_english_marker
=== RUN   TestClassify_HardCreditMarkers/existing_chinese_marker
=== RUN   TestClassify_HardCreditMarkers/credit_limit_reached
    client_test.go:26: Classify(403, "{\"error\":{\"message\":\"your credit limit has been reached\"}}") = client, want hard_credit
--- FAIL: TestClassify_HardCreditMarkers (0.00s)
    --- FAIL: TestClassify_HardCreditMarkers/credit_limit_reached (0.00s)
FAIL
```

Note that three subtests already pass. That is acceptable on the first run **only** because they are characterisation
cases pinned into the same table; the loop is driven by the fourth, which fails for the intended assertion reason.
If the whole table had passed, the test would be evidence that the behaviour already exists and no production change
would be justified.

#### Step 2 — GREEN: minimal production change

Add one marker to the existing list. No new function, no new branch, no change to priority order:

```go
// internal/upstream/client.go:61-67
var hardMarkers = []string{
	"insufficient credit", "no credit", "credit exhausted", "out of credit",
	"quota exceeded", "quota exhaust", "payment required", "credit not enough",
	"not enough credit", "credit balance is too low", "billing",
	"credit limit",                       // <-- the minimal change
	"积分不足", "额度不足", "余额不足", "积分用完", "额度用尽", "没有积分",
}
```

Re-run. All four subtests pass. Then run the whole package and the whole repo:

```bash
go test ./internal/upstream && go test ./...
```

#### Step 3 — REFACTOR

Now the test exists, so structure can be improved safely. Two legitimate refactors:

1. Extract the marker lists into named, documented slices with a comment stating the evidence grade
   (`PRIOR-ART`, never `VERIFIED` — see §7.2), because the marker vocabulary is a guess about upstream wording
   (`02-usage-semantics.md:321-325`).
2. De-duplicate the case-insensitive comparison helper used twice in `Classify` (`client.go:82` and `:87`):

```go
func containsFold(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
```

Refactor is only complete when `go test ./...`, `go test -race ./...` and `go vet ./...` are all green.

#### Step 4 — What this loop must NOT do

- It must not add a marker without a test case naming the exact body string.
- It must not widen a marker (e.g. `"credit"`) to make an unrelated test pass — over-broad markers classify 400s as
  credit exhaustion and disable a pool.
- It must not add a live upstream call "to confirm the wording". That is a `BLOCKED-CRED` item (§7.2), not a test.

#### Step 5 — Regression consequence

Because `Classify` is evaluated *before* the status code for 401 (`client.go:86-93`), adding `"credit limit"` also
means a `401` whose body contains that phrase now becomes `hard_credit` (12 h cooldown) instead of `session_dead`
(permanent disable). That is intentional and must be stated in the test table as an explicit case:

```go
{"marker beats 401", http.StatusUnauthorized, `{"error":{"message":"credit limit reached"}}`, ErrHardCredit},
```

A TDD loop that adds a marker without pinning this interaction is incomplete.

---

## 2. Test pyramid for this codebase

### 2.1 The pyramid, with real package status

```
                  ▲  manual smoke            (0 automated; requires user token)
                 ╱ ╲
                ╱   ╲  handler-level           (httptest + httptest.Server)      ← 0 tests today
               ╱─────╲
              ╱       ╲ integration             fake upstream httptest.Server    ← 0 tests today
             ╱─────────╲
            ╱           ╲ unit                  payload / sse / pool             ← 13 test funcs today
           ╱─────────────╲
```

| layer | packages | current state | what belongs here |
|---|---|---|---|
| **Unit** | `internal/upstream` (`payload.go`, `sse.go`), `internal/pool` (`pool.go`) | **exists**: 12 `Test*` + 1 `Benchmark*` | pure functions: model alias resolution, request translation, SSE frame parsing/aggregation, pool state transitions |
| **Handler-level** | `internal/server` (`handler.go`) via `httptest.NewRecorder` + `handler.ServeHTTP` | **does not exist** | routing, `withAuth` 401/200, `/healthz` body, `/v1/models` shape, 503 when pool is empty — all with a fake upstream |
| **Integration** | `internal/server` + `internal/upstream` against `httptest.NewServer` as fake upstream | **does not exist** | rotation across two accounts, cooldown triggered by a canned 402, token refresh before call, header selection (`x-api-key` vs Bearer) |
| **Manual smoke** | live `https://code.phanthy.com` | **forbidden by governance without a user token** | one-shot verification of `BLOCKED-CRED` items only (§7.2) |

### 2.2 Measured baseline (run 2026-10-26, `go1.26.5 linux/amd64`)

`go test -cover ./...`:

```
	phanthycode2api/cmd/login		coverage: 0.0% of statements
	phanthycode2api/cmd/server		coverage: 0.0% of statements
	phanthycode2api/internal/auth		coverage: 0.0% of statements
ok  	phanthycode2api/internal/pool	0.009s	coverage: 68.8% of statements
	phanthycode2api/internal/scheduler		coverage: 0.0% of statements
	phanthycode2api/internal/server		coverage: 0.0% of statements
ok  	phanthycode2api/internal/upstream	0.020s	coverage: 30.6% of statements
```

Per-function coverage inside the two tested packages (`go tool cover -func`), only the gaps quoted:

```
phanthycode2api/internal/upstream/client.go:76:		Classify		0.0%
phanthycode2api/internal/upstream/client.go:232:	RefreshToken		0.0%
phanthycode2api/internal/upstream/client.go:263:	CreateAPIKey		0.0%
phanthycode2api/internal/upstream/client.go:311:	EnsureAPIKey		0.0%
phanthycode2api/internal/upstream/client.go:334:	ChatStream		0.0%
phanthycode2api/internal/upstream/sse.go:123:		Stream			0.0%
phanthycode2api/internal/upstream/sse.go:188:		handleEvent		0.0%
phanthycode2api/internal/pool/pool.go:107:		SyncToDir		0.0%
phanthycode2api/internal/pool/pool.go:237:		List			0.0%
phanthycode2api/internal/upstream/payload.go:452:	BuildOpenAIStreamChunk	0.0%
total:							(statements)		38.3%
```

Interpretation, stated honestly:

- The **streaming half of the translator is untested**. `Stream` and `handleEvent` are at 0%. The SSE tests exercise
  `Aggregate` → `collectEvent`, which is a *different* code path with duplicated logic (`sse.go:333-383`). FR-02 and
  the stream half of FR-15 have no test.
- The **entire upstream client is untested**. `Classify` (FR-16), `RefreshToken` (FR-11), `ChatStream`,
  `EnsureAPIKey` (FR-13) are all 0%.
- **No HTTP surface is tested**. `internal/server` is 0%: FR-03, FR-04, FR-06, FR-17 have no test.
- `internal/auth` is 0%: the three-format `Parse` (FR-12 fallout) has no test.

---

## 3. Fixture strategy

### 3.1 Existing patterns to copy (with anchors)

| fixture kind | established pattern in this repo | anchor |
|---|---|---|
| **SSE golden stream** | A raw Go raw-string literal passed to `Aggregate(strings.NewReader(sse), "")`. The fixture includes `event:` lines that the parser deliberately ignores, and ends with `data: [DONE]`. | `internal/upstream/sse_test.go:10-30`, `:55-80`, `:116` |
| **Canned upstream JSON (request side)** | Inline JSON literal fed to `PrepareBody`, then re-unmarshalled into an anonymous struct that asserts only the fields under test. | `internal/upstream/payload_test.go:8-53`, `:55-94` |
| **Canned upstream JSON (tool-call side)** | `GetWeather` shape with `tool_calls` → `tool_use` → `tool_result` round trip. | `payload_test.go:121-158` |
| **Temp dirs for on-disk state** | `dir := t.TempDir()` then `fp := dir + "/state.json"`, write via the production API, re-open a fresh `New(fp)`. | `internal/pool/pool_test.go:90-107` |
| **Deterministic account construction** | `mkAuth(uid, apiKey)` helper returning `*auth.Auth` with only the fields the state machine reads. | `pool_test.go:10-12` |
| **Token-expiry control** | **Not yet established.** `auth.NeedsRefresh` calls `time.Now()` directly (`internal/auth/auth.go:37-42`), so expiry tests must set `ExpiresAt` relative to `time.Now()` (e.g. `time.Now().Add(5*time.Minute).Unix()`) rather than inject a clock. | `auth.go:36-42` (limitation) |

### 3.2 Rules for new fixtures

1. **Duplicated logic means duplicated fixtures.** Because `Stream`/`handleEvent` and `Aggregate`/`collectEvent` are
   separate implementations (`sse.go:188-276` vs `:333-383`), one SSE fixture must be asserted against **both**.
   A single golden file used by two tests is preferred over two hand-edited copies.
2. **Golden files, when a fixture exceeds ~40 lines**, live in `testdata/` next to the package
   (e.g. `internal/upstream/testdata/sse/basic_text.sse`). Under 40 lines, keep the literal inline — this matches the
   existing style and keeps the assertion readable.
3. **No real credentials, ever.** Fixture tokens are literal placeholders: `"access_token": "<token>"`,
   `"api_key": "<api_key>"`, `"refresh_token": "<token>"`. Never paste a value observed from a real account. This is
   the L2 rule from `00-governance.md:36-40` applied to test data.
4. **No `BLOCKED-CRED` body may be invented as a fixture.** Upstream success bodies for `/v1/messages` were never
   observed. A fixture that pretends to be one is a fabricated contract. The existing SSE fixture is legitimate because
   it is explicitly labelled PRIOR-ART (`02-usage-semantics.md:196-198`), and it is an Anthropic-shaped stream, not a
   claim about the target.
5. **Time-dependent behaviour**: prefer explicit duration inputs (`Cooldown(uid, kind, 1*time.Hour, "test")`) over
   clock injection. Where wall-clock is unavoidable, assert on `Status().Until` being "in the future" rather than on a
   precise timestamp.
6. **Temp dirs are the only allowed write location** in tests: `t.TempDir()`. A test must never touch `./auths`,
   `./data`, or the repository root.

---

## 4. Naming and structure conventions

### 4.1 Test names

```
Test<Unit>_<Behaviour>[_<Condition>]
```

Examples from the existing suite that already comply: `TestPick_PrefersAPIKey`, `TestCooldown_MakesUnhealthy`,
`TestNoteError_Threshold`, `TestPrepareBody_ToolChoiceStrings`, `TestAggregate_Basic`.

For new tests:

| target | name |
|---|---|
| `Classify` | `TestClassify_<MarkerOrStatus>` |
| `HandleEvent` / `Stream` | `TestStream_<EventType>` |
| `withAuth` | `TestWithAuth_RejectsMissingBearer` / `_AcceptsConfiguredKey` / `_DisabledWhenKeyEmpty` |
| rotation loop | `TestChatCompletions_RotatesOnHardCredit` |
| `Parse` | `TestParse_CLINativeFormat` / `_NestedFormat` / `_FlatFormat` |

Subtest names use lower-case words with spaces, e.g. `t.Run("marker beats 401", …)`.

### 4.2 Table-driven tests

Default shape for any function with more than one interesting input. Existing precedent:
`TestPrepareBody_ToolChoiceStrings` (`payload_test.go:96-119`) and `TestResolveModel` (`payload_test.go:167-194`).

```go
func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status int
		body   string
		want   ErrKind
	}{
		// one row per behaviour, name states the behaviour
	}
	for _, c := range cases {
		c := c // required if the Go version inside a subtest loop shadows differently
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := Classify(c.status, c.body); got != c.want {
				t.Errorf("Classify(%d, %q) = %s, want %s", c.status, c.body, got, c.want)
			}
		})
	}
}
```

Rules:
- The `name` field is mandatory; a numeric index is not an acceptable failure message.
- Assertion messages must print got and want **and** the input that produced them.
- No `t.Fatalf` inside a subtest table loop unless the failure makes subsequent rows meaningless.

### 4.3 `t.Parallel`

- Required on every test that touches no global state. `pool_test.go` is safe (`New("")` per test, no shared pool).
- Forbidden where the test mutates process state: anything writing a fixed file path (`.login-verifier`), anything
  calling `os.Chdir`, anything setting `os.Setenv`.
- Contract-level note: `pool.Pool` holds a `sync.RWMutex`, and `upstream.RefreshToken`/`CreateAPIKey` take
  `auth.Auth.mu`. Concurrency bugs in those locks are only found by `-race` with parallel callers, so a dedicated
  `TestPool_ConcurrentCooldownAndPick` with `t.Parallel()` + goroutines is required once handler tests land.

### 4.4 `t.TempDir`

- Mandatory for every test that exercises persistence (`pool.New(fp)`, `auth.SaveAtomic`, state reload).
- Pattern to copy verbatim from `pool_test.go:90-107`: write via production API, then construct a *new* instance with
  the same path and assert the reloaded state.
- Never use `os.MkdirTemp` directly — `t.TempDir()` auto-cleans and names the directory after the test.

### 4.5 `httptest`

Two distinct uses; do not conflate them.

**(a) `httptest.NewRecorder` — handler-level.** Drive `http.Handler` directly, no socket:

```go
func TestAuthMiddleware_RejectsWrongBearer(t *testing.T) {
	t.Parallel()
	h := NewHandler(Config{Pool: pool.New(""), APIKey: "test-key"})
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized { /* ... */ }
}
```

Note: `NewHandler` requires a `*upstream.Client`; pass `upstream.New("http://127.0.0.1:0")` so no real host can ever be
dialled by an accidental call.

**(b) `httptest.NewServer` — fake upstream.** Required for `RefreshToken`, `CreateAPIKey`, `ChatStream`, and the
rotation loop. The fake server must assert the *request* it received (headers, form fields), not only reply:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("x-api-key"); got != "<api_key>" {
		t.Errorf("x-api-key = %q", got)
	}
	w.Header().Set("Content-Type", "text/event-stream")
	io.WriteString(w, "data: {\"type\":\"message_start\",...}\n\n")
}))
defer srv.Close()

c := upstream.New(srv.URL) // Base field is overridable by design — client.go:109-141
```

`upstream.New(baseURL)` exists precisely so the base URL is injectable (`client.go:121-141`); tests must use it and
must never rely on the production default.

### 4.6 `-race` and `-cover`

- `go test -race ./...` is a **merge gate**, not an optional local nicety. Current baseline: passes (see §5).
- `go test -cover ./...` is reported, not gated per-package yet, because package coverage is 0% in four packages;
  a per-package threshold would fail immediately on unchanged code. The ratchet policy is in §5.3.

---

## 5. Commands and CI gates — with the actual observed baseline

All commands are copy-pasteable from `/workspace` and were executed for this document.

### 5.1 The command set

```bash
# 1. toolchain sanity
go version

# 2. build everything (must be zero output, exit 0)
go build ./...

# 3. the primary gate
go test ./...

# 4. race gate
go test -race ./...

# 5. vet gate
go vet ./...

# 6. format gate — prints files needing format; must produce NO output
gofmt -l .

# 7. test inventory (used for the traceability matrix)
go test -list '.*' ./...

# 8. coverage report
go test -cover ./...
go test -coverprofile=/tmp/cover.out ./internal/upstream ./internal/pool
go tool cover -func=/tmp/cover.out

# 9. run a single behaviour while in the red-green loop
go test -run 'TestClassify_HardCreditMarkers' -v ./internal/upstream

# 10. benchmark (existing: BenchmarkPrepareBody)
go test -bench=. -benchmem ./internal/upstream
```

### 5.2 Observed baseline, 2026-10-26 (`go version go1.26.5 linux/amd64`)

**`go test ./...` — exit 0:**

```
?   	phanthycode2api/cmd/login	[no test files]
?   	phanthycode2api/cmd/server	[no test files]
?   	phanthycode2api/internal/auth	[no test files]
ok  	phanthycode2api/internal/pool	(cached)
?   	phanthycode2api/internal/scheduler	[no test files]
?   	phanthycode2api/internal/server	[no test files]
ok  	phanthycode2api/internal/upstream	(cached)
```

**`go test -list '.*' ./...` — exit 0:**

```
?   	phanthycode2api/cmd/login	[no test files]
?   	phanthycode2api/cmd/server	[no test files]
?   	phanthycode2api/internal/auth	[no test files]
TestPick_PrefersAPIKey
TestPickExcluding_SkipsTried
TestCooldown_MakesUnhealthy
TestNoteError_Threshold
TestNoteSuccess_ResetsErrors
TestDisable
TestStatePersistence
ok  	phanthycode2api/internal/pool	0.004s
?   	phanthycode2api/internal/scheduler	[no test files]
?   	phanthycode2api/internal/server	[no test files]
TestPrepareBody_Basic
TestPrepareBody_Tools
TestPrepareBody_ToolChoiceStrings
TestPrepareBody_ToolResults
TestResolveModel
TestAggregate_Basic
TestAggregate_ToolCalls
TestAggregate_Empty
BenchmarkPrepareBody
ok  	phanthycode2api/internal/upstream	0.019s
```

**Test inventory: 15 test functions + 1 benchmark, across 2 of 7 packages.**

| package | test functions |
|---|---|
| `internal/pool` | 7 |
| `internal/upstream` | 8 (`TestPrepareBody_*` ×4, `TestResolveModel`, `TestAggregate_*` ×3) |
| `internal/auth`, `internal/scheduler`, `internal/server`, `cmd/login`, `cmd/server` | 0 — `[no test files]` |

**`go build ./...` — exit 0, no output.**

**`go test -race ./...` — exit 0:**

```
?   	phanthycode2api/cmd/login	[no test files]
?   	phanthycode2api/cmd/server	[no test files]
?   	phanthycode2api/internal/auth	[no test files]
ok  	phanthycode2api/internal/pool	1.019s
?   	phanthycode2api/internal/scheduler	[no test files]
?   	phanthycode2api/internal/server	[no test files]
ok  	phanthycode2api/internal/upstream	1.021s
```

**`go vet ./...` — exit 0, no output.**

**`gofmt -l .` — currently NOT clean.** Output (13 files):

```
cmd/login/main.go
cmd/server/config.go
cmd/server/main.go
internal/auth/auth.go
internal/pool/pool.go
internal/pool/pool_test.go
internal/scheduler/scheduler.go
internal/server/handler.go
internal/upstream/client.go
internal/upstream/payload.go
internal/upstream/payload_test.go
internal/upstream/sse.go
internal/upstream/sse_test.go
```

Cause, verified with `gofmt -d`: (a) every file lacks a trailing newline; (b) struct tag / inline-comment alignment
differs from gofmt's canonical columns (e.g. `internal/upstream/payload.go:27`, `cmd/server/config.go:22-26`).
This is cosmetic and does not affect `go build` or `go test`, but it means **a `gofmt -l` CI gate is RED at baseline**
and must be fixed (or the gate explicitly scoped to changed files) before it can be enforced.

**`go test -cover ./...` — exit 0:**

```
	phanthycode2api/cmd/login		coverage: 0.0% of statements
	phanthycode2api/cmd/server		coverage: 0.0% of statements
	phanthycode2api/internal/auth		coverage: 0.0% of statements
ok  	phanthycode2api/internal/pool	0.009s	coverage: 68.8% of statements
	phanthycode2api/internal/scheduler		coverage: 0.0% of statements
	phanthycode2api/internal/server		coverage: 0.0% of statements
ok  	phanthycode2api/internal/upstream	0.020s	coverage: 30.6% of statements
```

Combined statement coverage for the two tested packages: **38.3%** (`go tool cover -func` total).

### 5.3 Gate definitions and thresholds

| gate | command | threshold today | target threshold | status |
|---|---|---|---|---|
| G1 build | `go build ./...` | exit 0 | exit 0 | PASS |
| G2 unit | `go test ./...` | exit 0 | exit 0 | PASS |
| G3 race | `go test -race ./...` | exit 0 | exit 0 | PASS |
| G4 vet | `go vet ./...` | exit 0 | exit 0 | PASS |
| G5 format | `gofmt -l .` | must be empty | must be empty | **FAIL** (13 files) |
| G6 coverage — `internal/pool` | `go test -cover ./internal/pool` | no gate (68.8% baseline) | ≥ 68.8%, never decrease | baseline |
| G7 coverage — `internal/upstream` | `go test -cover ./internal/upstream` | no gate (30.6% baseline) | ≥ 60% once `Stream`/`Classify` tests land | baseline |
| G8 coverage — new packages | — | 0% | ≥ 70% per new test file's package | not yet applicable |
| G9 secret scan | see §5.4 | zero findings | zero findings | PASS |

Ratchet rule: thresholds are set to the **observed** value at the moment a package gains its first test, then may only
be raised. `internal/server` and `internal/auth` therefore get their threshold set on the day their first test lands,
not before.

### 5.4 Secret gate (must run before any merge)

```bash
# must print nothing
grep -rInE 'eyJ[A-Za-z0-9_-]{10,}|sk-[A-Za-z0-9]{10,}|refresh_token"[[:space:]]*:[[:space:]]*"[^<]' \
  --include='*.go' --include='*.md' --include='*.json' re/ internal/ cmd/ 2>/dev/null
```

Interpretation: any hit is a build-stopping defect. Fixture credentials must be `<token>` / `<api_key>` placeholders
only (§3.2 rule 3). This mirrors the M1/M2 verifier checks (`acceptance/M1.md:39`, `acceptance/M2.md:67`).

---

## 6. Definition of Done (DoD) checklist

A change is done only when **every** line below is true. Copy this block into the PR description.

### 6.1 Code
- [ ] Every new behaviour is reachable from a test that existed before it (or is labelled `characterisation` in the
      commit message, §8.1).
- [ ] No new production branch is unreachable from a test's input space, unless it is a documented defensive check.
- [ ] No new third-party dependency; `go.mod` still has zero `require` lines (`go.mod:1-3`).
- [ ] Public identifiers follow the existing style (exported = doc comment, sentence starting with the name).

### 6.2 Tests
- [ ] `go test ./...` exit 0 — pasted output in the PR.
- [ ] `go test -race ./...` exit 0.
- [ ] New test names follow `Test<Unit>_<Behaviour>` (§4.1).
- [ ] Table-driven with a `name` field for any multi-input test (§4.2).
- [ ] Any test touching persistence uses `t.TempDir()` (§4.4).
- [ ] Any test touching the upstream uses `httptest.NewServer` and asserts the outbound request, not only the reply (§4.5b).
- [ ] No test performs a network call to `code.phanthy.com` or any non-loopback host.
- [ ] Coverage number for the touched package quoted, and not lower than the §5.3 baseline.

### 6.3 Docs
- [ ] If a requirement's behaviour changed, `05-sdd-spec.md` §3/§4 text and the §10 traceability row are updated in the
      same change.
- [ ] If a new test was added, the matrix row changes from `NO-TEST` to `file::test` — no silent divergence.
- [ ] Any statement about upstream behaviour is graded (`VERIFIED` / `PRIOR-ART` / `INFERRED` / `BLOCKED-CRED`) per
      `01-endpoint-catalog.md:50-58`. Unverifiable claims stay `PRIOR-ART`.

### 6.4 No secret leakage
- [ ] `git diff` contains no real token, cookie, or API key value; only `<token>` / `<api_key>`.
- [ ] No new file under `auths/` or `data/` is staged (both are git-ignored; verify with `git status`).
- [ ] Secret gate G9 (§5.4) passes.
- [ ] No evidence file was modified to accommodate the change — evidence is append-only per milestone.

### 6.5 Endpoint / evidence updated
- [ ] If the gateway's own HTTP surface changed, `01-endpoint-catalog.md` is **not** the right place (it documents the
      *target*); record the gateway change in `05-sdd-spec.md` §6.1.
- [ ] If a `BLOCKED-CRED` item was resolved with a user token, the observation lands in a **new** milestone evidence
      file and the grade is upgraded there — never edited in place (`02-usage-semantics.md:459-467`).

---

## 7. Test-first checklist for the reverse-engineering deliverables

The RE track (M4/M5) produces an OpenAPI document, not Go code. The same loop applies with different artifacts.

### 7.1 OpenAPI change → contract test first

Before editing `re/apifox/openapi.json`:

- [ ] **Write the failing check first.** A contract test asserts the property that is about to change, e.g.
      "every path in `openapi.json` maps to an `EP-nn` in `01-endpoint-catalog.md`" or
      "no operation declares a security scheme not evidenced in the catalog".
- [ ] Confirm the check fails **for the intended reason** (missing operation), not because the file is unparseable.
- [ ] Apply the OpenAPI edit. Re-run: green.
- [ ] Structural gate: the document validates against OpenAPI 3.1 before merge.

Known contract tests this project must keep green (from the M2 acceptance gate, `acceptance/M2.md:130-161`):

| contract property | why | anchor |
|---|---|---|
| Exactly 17 operations, one per `EP-01…EP-17`, `operationId` unique | M2 verified 15/15; M6 added `phanthyModelsGet` + `modelStatsGet`; M6b validated 17/17 unique | `acceptance/M2.md:61-62`, `evidence/M6-index.md` |
| `messagesCreate` declares **two alternative** security schemes (`apiKeyAuth` + `bearerAuth`) and two response media types | scheme is `BLOCKED-CRED`; a single required scheme would assert unobserved behaviour | `acceptance/M2.md:138` |
| No success schema on `profileGet` / `claudeCliRolesGet` | bodies never observed | `acceptance/M2.md:154` |
| `webEventsCreate` is a placeholder with no request/response schema | semantics INFERRED-only | `acceptance/M2.md:144-145` |
| `clientDownloadGet` publishes **no** platform enum beyond `linux` | `darwin` proven rejected, others unknown | `acceptance/M2.md:146-148` |
| The six absent paths never appear as operations; no JSON 404 response object exists anywhere | target returns HTML 404 shells | `acceptance/M2.md:157-159`, `01-endpoint-catalog.md:443-462` |
| Model enum (if present) carries the description "PRIOR-ART / unverified live" | live catalogue unobtainable | `acceptance/M2.md:159-160` |

### 7.2 New endpoint → evidence fixture + catalog row, in that order

- [ ] **RED:** write the catalog row *and* the evidence anchor expected, with the observation left as
      `<pending: probe N>` so the row is visibly incomplete. Any check that "every catalog row has an existing
      evidence anchor" now fails.
- [ ] **Probe** (anonymous, ≤1 req/s, ≤20 calls per endpoint, GET/HEAD/OPTIONS only — `00-governance.md:20`):
      capture status, content type, byte count, header names, body digest.
- [ ] **GREEN:** store the artifact under `re/evidence/M<NN>-*` (never `/tmp`, never a credentials file), fill the anchor
      as `file:line`, and re-run the check.
- [ ] **Grade it** in exactly one of the four grades; if it needs a token, the row is `BLOCKED-CRED` and **no body is
      fabricated**.
- [ ] `POST` is only permitted for `POST /v1/messages` with the user's own token, ≤5 authenticated calls total
      (`00-governance.md:18-20`).

### 7.3 `BLOCKED-CRED` handling — the checklist that prevents invention

- [ ] The requirement / catalog row / OpenAPI operation exists and is marked `BLOCKED-CRED`.
- [ ] No example value, no sample body, no "expected" status code is written as if observed.
- [ ] The verification procedure is the one already specified in `02-usage-semantics.md:404-453` (C1–C5, ≤5 calls,
      abort conditions, `<token>` redaction) — not a newly invented probe plan.
- [ ] Abort discipline is restated: stop on `402`/`429`/`5xx`, on two consecutive `401`s, or on any body > ~1 MB.
- [ ] Results, when they exist, go into a **new** M5 evidence file; `01`/`02` stay byte-identical to what M2 delivered.

---

## 8. Anti-patterns and review checklist

### 8.1 Anti-patterns observed or likely in this repo

| # | anti-pattern | why it is harmful here | correct move |
|---|---|---|---|
| A1 | **Assert-the-implementation.** Re-unmarshalling production output into an anonymous struct and checking one field (as in `payload_test.go:35-52`) is fine, but copying that habit for SSE output without checking frame *ordering* and `[DONE]` misses the real contract. | FR-02's contract is the frame sequence, not a field value. | Assert the full emitted stream (split on `\n\n`), including the terminal `data: [DONE]`. |
| A2 | **Testing `Aggregate` and calling it "SSE tested".** | `Stream`/`handleEvent` are a separate implementation at 0% coverage. A green suite currently says nothing about FR-02. | Add `Stream` tests against the same fixtures; ideally refactor to one collector to remove the duplication. |
| A3 | **Fabricating a `BLOCKED-CRED` body as a fixture** to satisfy a handler test. | Creates a false contract with the upstream and violates governance L2. | Use an Anthropic-shaped fixture labelled PRIOR-ART, or test the handler with a stubbed upstream response the test itself authored. |
| A4 | **Sleeping to test cooldowns** (`time.Sleep(2*time.Second)`). | Slow, flaky, and the state machine uses wall-clock (`pool.go:173, 198`). | Pass an explicit short duration (`1*time.Nanosecond`) and assert the account is excluded, or assert `Status().Until` is after `now`. |
| A5 | **Skipping `t.Parallel` "because the pool is shared".** | Every existing pool test constructs its own `New("")`; there is no shared state. Suppressing parallelism hides races. | Add `t.Parallel()`; use `-race` to prove it. |
| A6 | **A test that never fails.** Adding a case whose input already classifies correctly, then claiming TDD. | Inflates coverage and confidence without behaviour change. | Confirm red first (§1.2), or label it `characterisation`. |
| A7 | **Over-broad markers** (`"credit"`, `"expired"`) to make a test pass. | Classifies unrelated 400s as credit exhaustion → 12 h cooling of healthy accounts. | Markers are specific phrases; each needs its own accepted body string. |
| A8 | **Formatting the whole tree as part of a feature PR.** | `gofmt -l .` lists 13 files today; a reformat would bury the real diff. | One dedicated formatting commit, reviewed alone, with no behaviour change. |
| A9 | **Leaking a real token into a test or fixture.** | Irreversible: it lands in git history. | `<token>` / `<api_key>` placeholders only; G9 gate (§5.4). |
| A10 | **Silent-skip tests** (`t.Skip` without a linked issue / requirement ID). | Coverage looks green while a requirement is untested. | If a test cannot be written, mark the requirement `NO-TEST` in the SDD matrix — do not silently skip. |

### 8.2 Reviewer checklist (per test-bearing PR)

- [ ] **Red evidence** in the PR description: the failing run's output, showing an assertion failure on the new
      behaviour (not a compile error).
- [ ] **Behaviour named, not implementation named**: `TestCooldown_MakesUnhealthy` (good) vs
      `TestCooldownSetsUntilField` (bad — pins internals).
- [ ] **Failure message is diagnostic**: prints got, want, and the input.
- [ ] **No test-only production API**: no exported function added solely so a test can call it. Prefer an in-package
      (`package upstream`) test file — which is how `sse_test.go` and `payload_test.go` are written.
- [ ] **No network**: grep the test file for `http://` / `https://` and confirm every occurrence is `httptest` or
      `127.0.0.1`.
- [ ] **No credentials**: G9 gate output pasted.
- [ ] **Requirement link**: the PR body names the FR/NFR ID(s) touched, and the matrix row in `05-sdd-spec.md` §10 is
      updated in the same commit.
- [ ] **Gate table**: `go test ./...`, `-race`, `go vet` outputs pasted; `gofmt -l .` either empty or unchanged from
      the baseline list in §5.2 (no *new* unformatted files).
- [ ] **Coverage direction**: touched package's percentage is ≥ its §5.3 baseline.

---

## 9. Appendix — requirement → test mapping recap

This table is the authoritative recap and **must agree** with `05-sdd-spec.md` §10. If the two ever diverge, the SDD
wins and this appendix is corrected in the same change.

| Req | Test status | Test anchor |
|---|---|---|
| FR-01 non-stream chat completion | partial | `internal/upstream/sse_test.go::TestAggregate_Basic`, `::TestAggregate_ToolCalls`, `::TestAggregate_Empty` (aggregation only; handler path untested) |
| FR-02 streaming chat completion | `NO-TEST` | — (`sse.go:Stream`, `handleEvent` at 0% coverage) |
| FR-03 models listing | `NO-TEST` | — (`internal/server` has no test files) |
| FR-04 health endpoint | `NO-TEST` | — |
| FR-05 pool status endpoint | partial | `internal/pool/pool_test.go::TestStatePersistence` (exercises `Status`; `List` untested) |
| FR-06 auth middleware | `NO-TEST` | — |
| FR-07 account rotation | partial | `pool_test.go::TestPick_PrefersAPIKey`, `::TestPickExcluding_SkipsTried` (pick preference / `tried` skipping only; handler rotation loop at 0%) |
| FR-08 hard-credit cooldown | partial | `pool_test.go::TestCooldown_MakesUnhealthy` (test never exercises `CoolHard`; `Cooldown` ignores `kind`) |
| FR-09 soft-rate cooldown | partial | `pool_test.go::TestCooldown_MakesUnhealthy` (exclusion asserted; the 60 s duration is not) |
| FR-10 error threshold + reset | yes | `pool_test.go::TestNoteError_Threshold`, `::TestNoteSuccess_ResetsErrors` |
| FR-11 token expiry refresh | `NO-TEST` | — (`client.go:RefreshToken` at 0%; `auth.NeedsRefresh` untested) |
| FR-12 PKCE login | `NO-TEST` | — (`cmd/login` has no test files) |
| FR-13 API-key fallback | `NO-TEST` | — (`EnsureAPIKey` / `ChatStream` header selection at 0%) |
| FR-14 keepalive scheduler | `NO-TEST` | — (`internal/scheduler` has no test files; `nextFire` is untested) |
| FR-15 SSE translation | partial | `sse_test.go::TestAggregate_Basic`, `::TestAggregate_ToolCalls`, `::TestAggregate_Empty` (aggregate path only) |
| FR-16 error classification | `NO-TEST` | — (`client.go:Classify` at 0%; the worked example in §1.3 is the first step to close this) |
| FR-17 all-accounts-unavailable 503 | `NO-TEST` | — |
| NFR-01 performance | `NO-TEST` | — (only `BenchmarkPrepareBody` exists, measuring translation, not end-to-end latency) |
| NFR-02..NFR-07 | `NO-TEST` | — (verified by inspection / governance review, not by test) |

Counts: 1 FR fully tested (FR-10), 6 partially tested (FR-01, FR-05, FR-07, FR-08, FR-09, FR-15), 10 FRs `NO-TEST`;
0 NFRs tested. That is **7/17 ≈ 41%** FR test-reach (fully covered 1/17 ≈ 6%) and **0/7** NFR test-reach — the same
figures as `05-sdd-spec.md` §10.

---

Document generated 2026-10-26. All baseline output in §5.2 was captured from `/workspace` with Go 1.26.5 on that date;
no network request was issued to `https://code.phanthy.com`, and no credential was read, copied or written.
Requirement IDs, the error taxonomy and the `BLOCKED-CRED` register are defined in `05-sdd-spec.md` §3, §4, §8 and §11;
§9 of this handbook is the test-side recap of that document's §10 traceability matrix and must change in lockstep with it.
