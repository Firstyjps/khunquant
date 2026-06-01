# Dev MCP — Implementation Progress

Head of engineering: Opus 4.8 (orchestrator)
Subagents: Sonnet 4.6

## Subtask Status

| # | Subtask | Status | Notes |
|---|---------|--------|-------|
| 1 | Config + defaults | ✅ done | struct + defaults verified, build passes |
| 2 | `pkg/debugtap` + loop hook | ✅ done | ring buffer + 3 record points in loop.go, build passes |
| 3 | `pkg/devmcp` (server/tools/redact) | ✅ done | 7 read-only tools, two-pass redaction, build passes |
| 4 | Gateway wiring + middleware | ✅ done | devmcp_api.go + registerDevMCP wired in setup+reload, build passes |
| 5 | WebUI + backend status endpoint | ✅ done | DebugSection toggle + /api/dev-mcp/status, TS+Go build pass |
| 6 | Tests + E2E verification | ✅ done | 27 tests pass (8 debugtap + 19 devmcp), race-clean |

---

## Subtask 1 — Config + defaults

**Owner:** Sonnet 4.6 subagent
**Files:** `pkg/config/config.go`, `pkg/config/defaults.go`
**Status:** 🔄 in-progress

### Validation checklist
- [ ] `DebugConfig` + `DevMCPConfig` struct added to `config.go` with correct json+env tags
- [ ] Nested correctly under top-level `Config` struct
- [ ] Defaults set in `defaults.go`: `Enabled:false`, `MaxLogEntries:50`, `PathPrefix:"/dev-mcp"`, `Token:""`
- [ ] No bind-host field (inherits gateway host)
- [ ] `make build` passes

---

## Subtask 2 — `pkg/debugtap` + loop hook

**Owner:** Sonnet 4.6 subagent
**Files:** `pkg/debugtap/store.go`, `pkg/agent/loop.go`
**Status:** 🔄 in-progress

### Validation checklist
- [ ] `pkg/debugtap/store.go`: `Store`, `Entry`, `NewStore`, `Record`, `List`, `Get`
- [ ] `Record` deep-copies `Messages` at capture time
- [ ] Per-message content capped at 8KB
- [ ] Ring wraps correctly at capacity
- [ ] `AgentLoop.debugTap` field + `SetDebugTap` added to `loop.go`
- [ ] One nil-checked `Record` call in `callLLM` (lines 1465-1494)
- [ ] `CloneMessages` helper in `pkg/debugtap`
- [ ] `make build` passes

---

## Subtask 3 — `pkg/devmcp` (server/tools/redact)

**Owner:** Sonnet 4.6 subagent
**Files:** `pkg/devmcp/server.go`, `pkg/devmcp/tools.go`, `pkg/devmcp/redact.go`
**Status:** ⏳ pending

### Validation checklist
- [ ] `mcp.NewServer` + `registerReadOnlyTools` (SOLE AddTool site)
- [ ] `NewHTTPHandler` with `JSONResponse:true`
- [ ] All 6 read-only tools registered (service_status, list_tools, list_llm_calls, read_llm_call, list_sessions, read_session_history, read_config)
- [ ] `redactConfig` handles plain-string secrets explicitly
- [ ] `redactPayload` applies FilterSensitiveData + plain-string scrub
- [ ] `make build` passes

---

## Subtask 4 — Gateway wiring + middleware

**Owner:** Sonnet 4.6 subagent
**Files:** `cmd/khunquant/internal/gateway/helpers.go`, `cmd/khunquant/internal/gateway/devmcp_api.go` (new)
**Status:** ⏳ pending

### Validation checklist
- [ ] `DebugTap` in `gatewayServices`
- [ ] `registerDevMCP` called in `setupAndStartServices` + reload path
- [ ] `bearerTokenMiddleware` uses constant-time compare
- [ ] `generateDevMCPToken` mirrors 24-byte hex pattern
- [ ] `al.SetDebugTap(nil)` called when disabled
- [ ] `make build` passes

---

## Subtask 5 — WebUI + backend status endpoint

**Owner:** Sonnet 4.6 subagent
**Files:** `web/frontend/src/components/config/form-model.ts`, `config-sections.tsx`, `config-page.tsx`, new backend status route
**Status:** ⏳ pending

### Validation checklist
- [ ] `debugDevMcpEnabled` in `CoreConfigForm`/`EMPTY_FORM`/`buildFormFromConfig`
- [ ] `<SwitchCardField>` in Debug group with warning hint
- [ ] Endpoint URL + token copy-button shown when enabled
- [ ] `patchAppConfig` payload sends `debug.dev_mcp.enabled` only (not token)
- [ ] `GET /api/dev-mcp/status` → `{enabled, endpoint, token}`
- [ ] TypeScript compiles

---

## Subtask 6 — Tests + E2E

**Owner:** Sonnet 4.6 subagent
**Status:** ⏳ pending

### Validation checklist
- [ ] `pkg/debugtap/store_test.go`: ring wrap, deep-copy, `-race`
- [ ] `pkg/devmcp/redact_test.go`: all plain-string + SecureString secrets redacted
- [ ] `pkg/devmcp/tools_test.go`: tool allowlist assertion
- [ ] `devmcp_api_test.go`: loopback 403, wrong-token 401, correct-token 200
- [ ] `make test` passes

---

## Review Log

*(Head engineer notes go here after each subtask review)*
