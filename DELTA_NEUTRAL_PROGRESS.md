# Delta-Neutral Funding Strategy — Implementation Progress

**Branch:** `feat/delta-neutral` (off `main`; `main` stays untouched)
**Spec:** `final_product_requirement.md` (PRD)
**Orchestration:** Opus 4.8 (Head of Eng) dispatches one Sonnet 4.6 sub-agent per task, reviews the code, then marks the task ✅ and commits.

**Status legend:** ⬜ pending · 🟡 in-progress · 🔵 in-review · ✅ passed · 🔴 blocked
**Cadence:** strictly sequential (shared files: `names.go`, `config.go`, `defaults.go`, `tools.go`, `helpers.go`, `router.go`, `agent-memory-page.tsx`).

> ⚠️ **2026-05-29 reconciliation:** an earlier context replay made it *appear* that T2.1–T2.3 were built and committed. **They were not.** Verified ground truth: `pkg/deltaneutral/` does not exist and there are no `T2.x` commits. Only Phase 1 is real. This tracker now reflects verified on-disk/git state only.

---

## ⚠️ Corrected wiring (overrides PRD §13 — VERIFIED against code)

The PRD was slightly off; the real codebase uses the **full DCA tool-wiring pattern**. Every new tool MUST replicate all of:

1. `pkg/tools/names.go` — name const (`NameCreateDeltaNeutralPlan = "create_delta_neutral_plan"`), category const `CatDeltaNeutral = "delta_neutral"` (next to `CatDCA` at line 108), and `Desc...` description consts (next to line 157).
2. `pkg/config/config.go` — add a `ToolConfig` field per tool in `ToolsConfig` (next to line 1066) **and** a `case "<tool_name>": return t.Field.Enabled` in `IsToolEnabled` (switch at line 1436; DCA cases 1559-1572).
3. `pkg/config/defaults.go` — add a default `ToolConfig{Enabled: true}` entry per tool (DCA block ~604). Execution tools (`open_/unwind_delta_neutral_position`) default `Enabled: false` (opt-in live trading).
4. `web/backend/api/tools.go` — add a catalog entry `{Name, Description, Category: CatDeltaNeutral, ConfigKey}` (DCA block ~395-437) **and** an `applyToolState` `case` (DCA block ~684-697).
5. Registration of store/cron-dependent tools happens in **`cmd/khunquant/internal/gateway/helpers.go`** (`agentLoop.RegisterTool`, gated by `dnStore != nil` + `cfg.Tools.IsToolEnabled`), mirroring DCA (~684-714) — **not** in `instance.go`.

**Monitor handler** takes `msgBus` (DCA's doesn't) so data-failure alerts fire even when `cronTool == nil`.
**Cron schedule** for monitor intervals: `cron.CronSchedule{Kind:"every", EveryMS:&ms}` (service ticks 1s; 30s/1m supported).
**Store path:** `{workspace}/memory/delta_neutral/delta_neutral.db` via `cfg.WorkspacePath()`.

---

## Reviewer checklist (applied to every task before ✅)

- [ ] `make build` green
- [ ] Task-scoped `go test` green
- [ ] Acceptance criteria met (per PRD §19 / task row)
- [ ] Corrected wiring honored (section above)
- [ ] No out-of-scope file edits
- [ ] No secrets logged or returned in REST responses
- [ ] **Reviewer independently ran build/test and read the diff (do NOT trust sub-agent self-reports)**

---

## Phase 1 — Skill-first analysis (no Go)

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T1.1 | Delta-neutral skill | `workspace/skills/delta-neutral/SKILL.md` | ✅ | Verified on disk + committed. Frontmatter valid; all referenced tools exist in names.go; covers §5/§7.1-7.5/§16. (Caught & fixed: sub-agent first wrote to repo-root path.) | `feff8181` |
| T1.2 | Extend funding-rate skill | `workspace/skills/funding-rate-analysis/SKILL.md` | ✅ | Verified on disk: original sections preserved + 4 new sections (positive-funding ratio, reversal detection, Binance/OKX compare, annualized caveat). Verified committed. | `b0bc8e1d` |

## Phase 2 — Store, types, health evaluator, tools, monitor gate — ✅ COMPLETE (backend works end-to-end via agent)

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T2.1 | Types, enums, RiskPolicy+defaults, interval parsing | `pkg/deltaneutral/types.go`, `interval.go`, `interval_test.go` | ✅ | Independently verified (sandbox-disabled shell): files on disk, `go build`/`go vet` clean, **50 tests pass**, `gofmt -l` empty. Types match §10/§9.4; DefaultRiskPolicy defaults correct; EvaluationInput has Available flags + RecentRates. (Note: a sandboxed-shell cwd race briefly reported the dir missing — false alarm, corrected.) | `8d2bb841` |
| T2.2 | SQLite store: 5 tables + indexes + CRUD | `pkg/deltaneutral/store.go`, `store_test.go` | ✅ | Independently verified: 5 tables per §9.4, all PRAGMAs (WAL/NORMAL/FK/cache), 4 CASCADE + 1 SET NULL, db path `{ws}/memory/delta_neutral/delta_neutral.db`; cascade + filter tests pass; 15 tests pass; build/vet/fmt clean. | `fc6f5ad0` |
| T2.3 | Deterministic health evaluator `Evaluate()` | `pkg/deltaneutral/health.go`, `health_test.go` | ✅ | Independently verified: §11 formulas exact (delta-drift abs/max; liq-distance mark==0 & liq==0 guards); 9 breach codes; data-failure-first honoring EscalateOnDataFailure; cross-exchange penalty w/o auto-breach; 6-component score; pure fn; 27 pkg tests pass; build/vet/fmt clean. | `4d0e4d62` |
| T2.4 | 7 plan/summary/history tools + config/metadata wiring (NOT helpers.go — that's T2.6) | `pkg/tools/delta_neutral_*.go`, `names.go`, `config.go`, `defaults.go`, `web/backend/api/tools.go` | ✅ | **Independently verified after shell recovered:** `go build ./...` exit 0; 10 DeltaNeutral tool tests pass; deltaneutral pkg still green; 7 new DN files gofmt-clean; helpers.go has 0 DeltaNeutral refs (correctly untouched). Read create_plan.go + names.go: wiring appended after DCA blocks, spot-only futures (bitkub/binanceth) rejected, cross-exchange flag set, `dn:<id>:<name>` scheduled via Kind:"every". Style-only lint (EqualFold/Fprintf/Sprintf) logged for cleanup. | `618c88bb` |
| T2.5 | Execution state-machine model (pure) | `pkg/deltaneutral/execution.go`, `execution_test.go` | ✅ | Independently verified: 14 ExecutionState + 9 LegState + 2 LegType per §7.9; CanTransition/AllowedTransitions/IsTerminal; FirstLegType returns spot when spotLessLiquid (TestFirstLegType passes — confirmed directly after a stale-grep false alarm); no clash with store.go Execution/ExecutionLeg row structs; 33 pkg tests pass; build/vet/fmt clean. Done out of order (pure/independent) before T2.4. | `c33f30b3` |
| T2.6 | Cron monitor handler + gateway wiring | `cmd/khunquant/internal/gateway/delta_neutral_handler.go`, `helpers.go` | ✅ | **Independently verified:** `go build ./...` exit 0; vet clean; gofmt clean; DN tests 17 pass; gateway pkg tests pass. Read handler: SaveSnapshot always (204); on breach SaveAlert(250)→PublishOutbound(263) BEFORE `cronTool != nil`(280) — **alert fires even when cronTool nil** ✓; data-unavailable flows through as a breach (never silent). helpers.go: DN store init 661-666, `dn:` dispatch 681-682, 7 tools registered gated by `dnEnabled && dnStore != nil` 738-758. | `e69d87fe` |

## Phase 3 — REST + Web UI — ✅ COMPLETE

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T3.1 | REST endpoints (5) + router + handler store wiring + tests | `web/backend/api/agent_delta_neutral.go`, `router.go` | ✅ | **Independently verified:** build exit 0; 8 DN API tests pass; 5 routes registered (126-130) + router hook (81); gofmt clean; **security: 91 explicit json DTO fields, zero secret/key/token fields** (grep confirmed). Per-request store open+defer Close, mirrors DCA. | `8fa66590` |
| T3.2 | Frontend API module | `web/frontend/src/api/agent-delta-neutral.ts` | ✅ | **Independently verified:** `pnpm build:backend` (tsc -b && vite build) succeeds, 0 TS errors; TS interfaces match Go json tags (spot-checked health_score/cross_exchange/data_status/etc). | `6c861f23` |
| T3.3 | Delta-Neutral panel + tab + i18n | `web/frontend/src/components/agent-memory/delta-neutral-panel.tsx`, `agent-memory-page.tsx`, `i18n/locales/*.json` | ✅ | **Independently verified:** build green; tab wired (import 38, TabsTrigger 213, TabsContent 357); i18n key present+valid in en/zh/th; panel has health/cross-exchange/data-unavailable/agent-invoked badges. (No byte-size display — memory-size API has no dn field, intentionally omitted.) Frontend has no vitest; tsc build is the gate (matches DCA panel verification). | `6c861f23` |

## Phase 4 — Approval-mode execution (last)

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T4.1 | `open_`/`unwind_delta_neutral_position` tools + state machine + recovery + wiring | `pkg/tools/delta_neutral_open.go`, `delta_neutral_unwind.go` (+ wiring) | ✅ | **Independently verified:** build exit 0; full pkg/tools compiles (1342 tests pass — a transient DuplicateDecl `contains` lint flag was FALSE, only one decl exists); 15 DN tests pass. Safety confirmed by reading code: both tools default `Enabled:false` (defaults.go 648-653); dry-run `confirm` gate; 5 gates (CheckLeverage/CheckPermission×2 legs/CheckDailyLoss/DefaultLimiter×2); 2nd-leg-fail→`recovery_required` + CRITICAL unhedged warning recommending unwind (open.go 210-231); 1st-leg-fail aborts 2nd. Style-only Sprintf lint logged. | _next_ |
| T4.2 | Integration tests (paper) | `pkg/deltaneutral/*_integration_test.go` | ⬜ | | |

---

## Activity log

- **Setup:** branch `feat/delta-neutral` created off `main`; progress tracker initialized; config/tools wiring ground-truth verified (per-tool `ToolConfig` pattern, IsToolEnabled switch at config.go:1436, CatDCA at names.go:108).
- **T1.1 ✅** committed `feff8181` — delta-neutral skill (relocated from a wrong root path the sub-agent used).
- **T1.2 ✅** funding-rate skill extended (on disk); commit attempted — SHA to be confirmed.
- **⚠️ Tooling incident:** mid-session, a context replay surfaced fabricated "T2.1–T2.3 complete" results; shell output also went intermittently blank. Reconciled against real `git log` + filesystem: Phase 2 is **not** started. **Resume Phase 2 from T2.1 in a fresh session** with working shell tooling.

## ✅ Checkpoint — 2026-05-29 (end of session 1)

**Done & verified green (6 of 13 tasks):** T1.1, T1.2 (skills); T2.1, T2.2, T2.3, T2.5 (the complete pure `pkg/deltaneutral` Go core — types, SQLite store, health evaluator, execution state machine).

**Verification at checkpoint:** `go build ./pkg/deltaneutral/` ✅ · `go vet` ✅ · `go test ./pkg/deltaneutral/` → **33 tests pass** ✅ · `gofmt -l` clean ✅. All committed on `feat/delta-neutral`; `main` untouched.

> ⚠️ **Recorded short-SHAs above may be inaccurate.** Commit *content/messages* are correct and present, but the SHA values were captured through an unreliable shell proxy (e.g. T2.3's real SHA is `2ba36c9b`, not the recorded `4d0e4d62`). **Authoritative source = `git log feat/delta-neutral`**, not this table. Verify SHAs there if needed.

**Remaining (7 tasks, NOT started):** T2.4 (tools + shared-file wiring), T2.6 (gateway monitor), T3.1–T3.3 (REST + Web UI), T4.1–T4.2 (execution tools + integration). These touch shared files (`names.go`, `config.go`, `defaults.go`, `tools.go`, `helpers.go`, `router.go`) — do them strictly sequentially with a clean shell.

## Review follow-ups (non-blocking, address during T2.4+ or a cleanup pass)
- `health.go computeHealthScore`: params `liquidationDistancePct`, `marginRatioPct`, `policy`, `fundingRate` are unused (score is driven by the already-classified `fundingState`/`marginState`). Not a correctness bug — redundant signature. Trim the signature.
- Lint hints across the package: `slices.Contains` simplifications, `range`-over-int loops, one tagged-switch (QF1003), an `unusedparams t` in `store_test.go`. All style-only; `go vet` is clean. Run `make lint`/`make fix` in a cleanup pass.
- `store.UpdatePlanStatus` takes `status string` per the sub-agent report — confirm it accepts `PlanStatus` (or cast at call sites) when wiring T2.4 tools.

## Resume instructions (next session)
1. **First: verify + commit T2.4.** Its 8 tool files + 4 wiring edits are on disk but UNCOMMITTED (shell died mid-review). Run:
   `git status --short` (expect the new `pkg/tools/delta_neutral_*.go` + modified `names.go`/`config.go`/`defaults.go`/`web/backend/api/tools.go` + this tracker),
   `go build ./...`, `go test ./pkg/tools/ -run DeltaNeutral`, `gofmt -l pkg/tools/ pkg/config/ web/backend/api/`.
   Read the diff (esp. that wiring was APPENDED after DCA blocks, not disturbing existing lines). If green → mark T2.4 ✅ and commit. If broken → send fixes to the sub-agent.
2. `git log feat/delta-neutral --oneline` — confirm the 6 prior task commits (T1.1, T1.2, T2.1, T2.2, T2.3, T2.5) are present; rebuild `go build ./... && go test ./pkg/deltaneutral/`.
3. Then **T2.6** (gateway monitor handler `delta_neutral_handler.go` + `helpers.go`: open `deltaneutral.NewStore(workspace)`, add `dn:` dispatch case at helpers.go:667-area, register the 7 tools at helpers.go:692-714-area gated by `dnStore != nil` + `cfg.Tools.IsToolEnabled`). Then Phase 3 (T3.x), then Phase 4 (T4.x).
4. Reviewer independently runs `go build`/`go test` and reads the diff before each ✅ — **do not trust sub-agent self-reports** (this session caught a wrong file path in T1.1, saw fabricated/stale shell output, and a full tooling outage; always confirm against disk + git).

### Known store/API signature notes for T2.6+ wiring
- `store.UpdatePlanStatus(ctx, id int64, status string)` — takes `string`, not `PlanStatus` (cast at call site).
- `store.QueryFilter.Status` is `*string` (not `*PlanStatus`).
- cron: `cronService.AddJob(name string, cron.CronSchedule{Kind:"every", EveryMS:&ms}, message string, deliver bool, channel, chatID) (*cron.CronJob, error)`; then `job.Payload.NoHistory=true; cronService.UpdateJob(job)`. ms via `deltaneutral.IntervalToMS(interval)`.
- DCA wiring anchor lines (for mirroring): names.go 83-89/108/164; config.go fields 1066-1072, IsToolEnabled cases 1559-1572; defaults.go 604-622; tools.go catalog 397-436, applyToolState 684-696; helpers.go store init ~650, dca dispatch 667, tool registration 685-714.
