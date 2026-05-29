# Delta-Neutral Funding Strategy — Implementation Progress

**Branch:** `feat/delta-neutral` (off `main`; `main` stays untouched)
**Spec:** `final_product_requirement.md` (PRD)
**Orchestration:** Opus 4.8 (Head of Eng) dispatches one Sonnet 4.6 sub-agent per task, reviews the code, then marks the task ✅ and commits.

**Status legend:** ⬜ pending · 🟡 in-progress · 🔵 in-review · ✅ passed · 🔴 blocked
**Cadence:** strictly sequential (shared files: `names.go`, `config.go`, `defaults.go`, `tools.go`, `helpers.go`, `router.go`, `agent-memory-page.tsx`).

---

## ⚠️ Corrected wiring (overrides PRD §13 — VERIFIED against code)

The PRD was slightly off; the real codebase uses the **full DCA tool-wiring pattern**. Every new tool MUST replicate all of:

1. `pkg/tools/names.go` — name const (`NameCreateDeltaNeutralPlan = "create_delta_neutral_plan"`), category const `CatDeltaNeutral = "delta_neutral"` (next to `CatDCA` at line 108), and `Desc...` description consts (next to line 157).
2. `pkg/config/config.go` — add a `ToolConfig` field per tool in `ToolsConfig` (next to line 1066) **and** a `case "<tool_name>": return t.Field.Enabled` in `IsToolEnabled` (switch at line 1435; DCA cases ~1559-1572).
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

---

## Phase 1 — Skill-first analysis (no Go)

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T1.1 | Delta-neutral skill (scan, funding analysis, portfolio-per-leg, same-exchange rec, round-trip breakeven, no-exec-without-confirm) | `workspace/skills/delta-neutral/SKILL.md` | ✅ | Frontmatter valid (verified raw); all 10 referenced tools exist in names.go; covers §5/§7.1-7.4/§16; round-trip breakeven + cross-exchange warning present | _pending_ |
| T1.2 | Extend funding-rate skill (positive-funding ratio, reversal detection, Binance/OKX compare, annualized caveat) | `workspace/skills/funding-rate-analysis/SKILL.md` | ⬜ | | |

## Phase 2 — Store, types, health evaluator, tools, monitor gate

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T2.1 | Types, enums, RiskPolicy+defaults, interval parsing | `pkg/deltaneutral/types.go`, `interval.go`, `interval_test.go` | ⬜ | | |
| T2.2 | SQLite store: 5 tables + indexes + CRUD/snapshots/alerts/executions | `pkg/deltaneutral/store.go`, `store_test.go` | ⬜ | | |
| T2.3 | Deterministic health evaluator `Evaluate()` | `pkg/deltaneutral/health.go`, `health_test.go` | ⬜ | | |
| T2.4 | 7 plan/summary/history tools + full wiring (names/config/defaults/tools.go) | `pkg/tools/delta_neutral_*.go`, `names.go`, `config.go`, `defaults.go`, `web/backend/api/tools.go` | ⬜ | | |
| T2.5 | Execution state-machine model (pure) | `pkg/deltaneutral/execution.go`, `execution_test.go` | ⬜ | | |
| T2.6 | Cron monitor handler + gateway wiring (store init, `dn:` dispatch, tool registration) | `cmd/khunquant/internal/gateway/delta_neutral_handler.go`, `helpers.go` | ⬜ | | |

## Phase 3 — REST + Web UI

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T3.1 | REST endpoints (5) + router + handler store wiring + tests | `web/backend/api/agent_delta_neutral.go`, `router.go` | ⬜ | | |
| T3.2 | Frontend API module | `web/frontend/src/api/agent-delta-neutral.ts` | ⬜ | | |
| T3.3 | Delta-Neutral panel + tab + i18n | `web/frontend/src/components/agent-memory/delta-neutral-panel.tsx`, `agent-memory-page.tsx`, `i18n/locales/*.json` | ⬜ | | |

## Phase 4 — Approval-mode execution (last)

| Task | Description | Files | Status | Reviewer notes | Commit |
|------|-------------|-------|--------|----------------|--------|
| T4.1 | `open_`/`unwind_delta_neutral_position` tools + state machine + recovery + wiring | `pkg/tools/delta_neutral_open.go`, `delta_neutral_unwind.go` (+ wiring files) | ⬜ | | |
| T4.2 | Integration tests (paper): create→monitor, breach→alert, exec success, one-leg-fail→recovery | `pkg/deltaneutral/*_integration_test.go` | ⬜ | | |

---

## Activity log

- Setup: branch `feat/delta-neutral` created; progress tracker initialized; config/tools wiring ground-truth verified (per-tool `ToolConfig` pattern confirmed).
