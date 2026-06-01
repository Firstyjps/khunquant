package devmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cryptoquantumwave/khunquant/pkg/logger"
)

// registerReadOnlyTools registers all read-only developer tools with the MCP server.
func registerReadOnlyTools(s *mcp.Server, d Deps) {
	// Tool 1: service_status — no parameters
	s.AddTool(&mcp.Tool{
		Name:        "service_status",
		Description: "Get service status: enabled providers, current model, registered agents",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, serviceStatusHandler(d))

	// Tool 2: list_tools — AgentID parameter
	s.AddTool(&mcp.Tool{
		Name:        "list_tools",
		Description: "List all available tools registered with an agent",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"Agent ID (default: 'main')"}
			}
		}`),
	}, listToolsHandler(d))

	// Tool 3: list_llm_calls — Limit parameter
	s.AddTool(&mcp.Tool{
		Name:        "list_llm_calls",
		Description: "List recent LLM calls (metadata only, no message content)",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"limit":{"type":"integer","description":"Maximum number of entries to return (0=all)"}
			}
		}`),
	}, listLLMCallsHandler(d))

	// Tool 4: read_llm_call — Seq parameter
	s.AddTool(&mcp.Tool{
		Name:        "read_llm_call",
		Description: "Read full details of a specific LLM call (including redacted message content)",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"seq":{"type":"integer","description":"Sequence number of the call"}
			},
			"required":["seq"]
		}`),
	}, readLLMCallHandler(d))

	// Tool 5: list_sessions — AgentID parameter
	s.AddTool(&mcp.Tool{
		Name:        "list_sessions",
		Description: "List all session keys for an agent",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"Agent ID (default: 'main')"}
			}
		}`),
	}, listSessionsHandler(d))

	// Tool 6: read_session_history — AgentID and SessionKey parameters
	s.AddTool(&mcp.Tool{
		Name:        "read_session_history",
		Description: "Read conversation history for a session with grep/tail/head filtering. Use 'tail' for the most recent messages, 'contains' to grep, 'role' to filter by speaker.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"Agent ID (default: 'main')"},
				"session_key":{"type":"string","description":"Session key"},
				"tail":{"type":"integer","description":"Return last N messages (like tail -n). Applied after role/contains filters."},
				"head":{"type":"integer","description":"Return first N messages (like head -n). Applied after role/contains filters."},
				"offset":{"type":"integer","description":"Skip first N messages before applying head/tail."},
				"role":{"type":"string","description":"Filter by role: 'user', 'assistant', or 'tool'"},
				"contains":{"type":"string","description":"Grep: only return messages whose content contains this string (case-insensitive)"}
			},
			"required":["session_key"]
		}`),
	}, readSessionHistoryHandler(d))

	// Tool 7: search_sessions — grep across all sessions
	s.AddTool(&mcp.Tool{
		Name:        "search_sessions",
		Description: "Grep across all sessions for an agent. Returns matching messages with their session key and index.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"Agent ID (default: 'main')"},
				"contains":{"type":"string","description":"Text to search for (case-insensitive)"},
				"role":{"type":"string","description":"Filter by role: 'user', 'assistant', or 'tool'"},
				"limit":{"type":"integer","description":"Max results to return (default: 20)"}
			},
			"required":["contains"]
		}`),
	}, searchSessionsHandler(d))

	// Tool 8: read_logs — gateway log lines with grep/tail/head
	s.AddTool(&mcp.Tool{
		Name:        "read_logs",
		Description: "Read gateway log lines (like the WebUI Logs page). Supports tail/head/grep/level filters. Long field values (base64 etc.) are truncated to 8 chars.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"tail":{"type":"integer","description":"Return last N lines (default: 50 when no filter set)"},
				"head":{"type":"integer","description":"Return first N lines"},
				"offset":{"type":"integer","description":"Skip first N lines before applying head/tail"},
				"contains":{"type":"string","description":"Grep: only return lines containing this string (case-insensitive)"},
				"level":{"type":"string","description":"Filter by log level: INF, WRN, ERR, DBG"}
			}
		}`),
	}, readLogsHandler(d))

	// Tool 9: read_config — no parameters
	s.AddTool(&mcp.Tool{
		Name:        "read_config",
		Description: "Read the full service configuration (all secrets masked)",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, readConfigHandler(d))
}

// serviceStatusHandler returns current service status.
func serviceStatusHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		registry := d.Loop.GetRegistry()
		agentIDs := registry.ListAgentIDs()

		status := map[string]interface{}{
			"timestamp":       time.Now().UTC().Format(time.RFC3339),
			"agents":          agentIDs,
			"agent_count":     len(agentIDs),
			"debug_tap_ready": d.DebugTap != nil,
		}

		// Add provider and model info from default agent
		if defaultAgent := registry.GetDefaultAgent(); defaultAgent != nil {
			status["model"] = defaultAgent.Model
		}

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: mustJSON(status),
				},
			},
		}

		return result, nil
	}
}

// listToolsHandler lists all tools for a given agent.
func listToolsHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}

		if input.AgentID == "" {
			input.AgentID = "main"
		}

		registry := d.Loop.GetRegistry()
		agent, ok := registry.GetAgent(input.AgentID)
		if !ok {
			return errorResult("agent not found: " + input.AgentID), nil
		}

		// Get tool definitions (as map[string]any from the schema)
		defs := agent.Tools.GetDefinitions()
		toolList := make([]map[string]interface{}, 0, len(defs))
		for _, def := range defs {
			toolItem := map[string]interface{}{
				"name": def["name"],
			}
			if desc, ok := def["description"]; ok {
				toolItem["description"] = desc
			}
			toolList = append(toolList, toolItem)
		}

		output := map[string]interface{}{
			"agent_id":   input.AgentID,
			"tool_count": len(toolList),
			"tools":      toolList,
		}

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: mustJSON(output),
				},
			},
		}

		return result, nil
	}
}

// listLLMCallsHandler returns metadata (without payloads) of recent LLM calls.
func listLLMCallsHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.DebugTap == nil {
			return errorResult("debug tap not available"), nil
		}

		var input struct {
			Limit int `json:"limit"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}

		entries := d.DebugTap.List(input.Limit)

		// Convert to metadata-only format (no message content)
		metadataList := make([]map[string]interface{}, 0, len(entries))
		for _, entry := range entries {
			metadata := map[string]interface{}{
				"seq":        entry.Seq,
				"timestamp":  entry.Timestamp.Format(time.RFC3339),
				"agent_id":   entry.AgentID,
				"session":    entry.SessionKey,
				"provider":   entry.Provider,
				"model":      entry.Model,
				"duration_ms": entry.DurationMS,
				"error":      entry.Err,
			}
			metadataList = append(metadataList, metadata)
		}

		output := map[string]interface{}{
			"count":   len(metadataList),
			"entries": metadataList,
		}

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: mustJSON(output),
				},
			},
		}

		return result, nil
	}
}

// readLLMCallHandler returns the full entry for a specific LLM call, with redacted content.
func readLLMCallHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if d.DebugTap == nil {
			return errorResult("debug tap not available"), nil
		}

		var input struct {
			Seq uint64 `json:"seq"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}

		entry, ok := d.DebugTap.Get(input.Seq)
		if !ok {
			return errorResult(fmt.Sprintf("entry not found: seq=%d", input.Seq)), nil
		}

		// Build output with redacted content
		output := map[string]interface{}{
			"seq":         entry.Seq,
			"timestamp":   entry.Timestamp.Format(time.RFC3339),
			"agent_id":    entry.AgentID,
			"session":     entry.SessionKey,
			"provider":    entry.Provider,
			"model":       entry.Model,
			"duration_ms": entry.DurationMS,
			"error":       entry.Err,
		}

		// Add redacted messages
		redactedMessages := make([]map[string]interface{}, 0, len(entry.Messages))
		for _, msg := range entry.Messages {
			redactedMsg := map[string]interface{}{
				"role": msg.Role,
			}

			// Redact content
			if msg.Content != "" {
				redactedMsg["content"] = redactPayload(msg.Content, d.Cfg)
			}

			// Redact tool calls if present
			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]map[string]interface{}, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					toolCall := map[string]interface{}{
						"id":   tc.ID,
						"name": tc.Name,
					}
					// Arguments is map[string]any; redact it as JSON
					if len(tc.Arguments) > 0 {
						argsJSON, _ := json.Marshal(tc.Arguments)
						toolCall["arguments"] = redactPayload(string(argsJSON), d.Cfg)
					}
					toolCalls = append(toolCalls, toolCall)
				}
				redactedMsg["tool_calls"] = toolCalls
			}

			redactedMessages = append(redactedMessages, redactedMsg)
		}
		output["messages"] = redactedMessages

		// Add redacted response if present
		if entry.Response != nil {
			redactedResponse := map[string]interface{}{
				"finish_reason": entry.Response.FinishReason,
			}

			// Redact content
			if entry.Response.Content != "" {
				redactedResponse["content"] = redactPayload(entry.Response.Content, d.Cfg)
			}

			// Redact tool calls in response
			if len(entry.Response.ToolCalls) > 0 {
				toolCalls := make([]map[string]interface{}, 0, len(entry.Response.ToolCalls))
				for _, tc := range entry.Response.ToolCalls {
					toolCall := map[string]interface{}{
						"id":   tc.ID,
						"name": tc.Name,
					}
					// Arguments is map[string]any; redact as JSON
					if len(tc.Arguments) > 0 {
						argsJSON, _ := json.Marshal(tc.Arguments)
						toolCall["arguments"] = redactPayload(string(argsJSON), d.Cfg)
					}
					toolCalls = append(toolCalls, toolCall)
				}
				redactedResponse["tool_calls"] = toolCalls
			}

			output["response"] = redactedResponse
		}

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: mustJSON(output),
				},
			},
		}

		return result, nil
	}
}

// listSessionsHandler returns all session keys for an agent.
func listSessionsHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			AgentID string `json:"agent_id"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}

		if input.AgentID == "" {
			input.AgentID = "main"
		}

		registry := d.Loop.GetRegistry()
		agent, ok := registry.GetAgent(input.AgentID)
		if !ok {
			return errorResult("agent not found: " + input.AgentID), nil
		}

		// Get session keys from the SessionStore
		sessionKeys := agent.Sessions.ListSessions()

		output := map[string]interface{}{
			"agent_id":    input.AgentID,
			"session_count": len(sessionKeys),
			"sessions":    sessionKeys,
		}

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: mustJSON(output),
				},
			},
		}

		return result, nil
	}
}

// readSessionHistoryHandler returns conversation history for a session with
// grep/tail/head/offset/role filtering so large sessions remain navigable.
func readSessionHistoryHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			AgentID    string `json:"agent_id"`
			SessionKey string `json:"session_key"`
			Tail       int    `json:"tail"`
			Head       int    `json:"head"`
			Offset     int    `json:"offset"`
			Role       string `json:"role"`
			Contains   string `json:"contains"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}
		if input.AgentID == "" {
			input.AgentID = "main"
		}
		if input.SessionKey == "" {
			return errorResult("session_key is required"), nil
		}

		registry := d.Loop.GetRegistry()
		ag, ok := registry.GetAgent(input.AgentID)
		if !ok {
			return errorResult("agent not found: " + input.AgentID), nil
		}

		history := ag.Sessions.GetHistory(input.SessionKey)
		summary := ag.Sessions.GetSummary(input.SessionKey)
		totalMessages := len(history)

		// Build redacted message list
		all := make([]map[string]interface{}, 0, len(history))
		for i, msg := range history {
			m := map[string]interface{}{"role": msg.Role, "index": i}
			if msg.Content != "" {
				m["content"] = redactPayload(msg.Content, d.Cfg)
			}
			if len(msg.ToolCalls) > 0 {
				tcs := make([]map[string]interface{}, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
					t := map[string]interface{}{"id": tc.ID, "name": tc.Name}
					if len(tc.Arguments) > 0 {
						argsJSON, _ := json.Marshal(tc.Arguments)
						t["arguments"] = redactPayload(string(argsJSON), d.Cfg)
					}
					tcs = append(tcs, t)
				}
				m["tool_calls"] = tcs
			}
			all = append(all, m)
		}

		// Apply offset
		if input.Offset > 0 && input.Offset < len(all) {
			all = all[input.Offset:]
		}

		// Apply role filter
		if input.Role != "" {
			filtered := all[:0]
			for _, m := range all {
				if strings.EqualFold(fmt.Sprintf("%v", m["role"]), input.Role) {
					filtered = append(filtered, m)
				}
			}
			all = filtered
		}

		// Apply contains filter (grep)
		if input.Contains != "" {
			needle := strings.ToLower(input.Contains)
			filtered := all[:0]
			for _, m := range all {
				haystack := strings.ToLower(fmt.Sprintf("%v %v", m["content"], m["tool_calls"]))
				if strings.Contains(haystack, needle) {
					filtered = append(filtered, m)
				}
			}
			all = filtered
		}

		// Apply head / tail
		switch {
		case input.Tail > 0 && input.Tail < len(all):
			all = all[len(all)-input.Tail:]
		case input.Head > 0 && input.Head < len(all):
			all = all[:input.Head]
		}

		output := map[string]interface{}{
			"agent_id":       input.AgentID,
			"session_key":    input.SessionKey,
			"total_messages": totalMessages,
			"returned":       len(all),
			"summary":        redactPayload(summary, d.Cfg),
			"history":        all,
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: mustJSON(output)}},
		}, nil
	}
}

// searchSessionsHandler greps across all sessions for matching messages.
func searchSessionsHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			AgentID  string `json:"agent_id"`
			Contains string `json:"contains"`
			Role     string `json:"role"`
			Limit    int    `json:"limit"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}
		if input.AgentID == "" {
			input.AgentID = "main"
		}
		if input.Contains == "" {
			return errorResult("contains is required"), nil
		}
		if input.Limit <= 0 {
			input.Limit = 20
		}

		registry := d.Loop.GetRegistry()
		ag, ok := registry.GetAgent(input.AgentID)
		if !ok {
			return errorResult("agent not found: " + input.AgentID), nil
		}

		needle := strings.ToLower(input.Contains)
		type match struct {
			SessionKey string      `json:"session_key"`
			Index      int         `json:"index"`
			Role       string      `json:"role"`
			Snippet    string      `json:"snippet"`
		}
		var matches []match

		for _, key := range ag.Sessions.ListSessions() {
			for i, msg := range ag.Sessions.GetHistory(key) {
				if input.Role != "" && !strings.EqualFold(msg.Role, input.Role) {
					continue
				}
				content := redactPayload(msg.Content, d.Cfg)
				if !strings.Contains(strings.ToLower(content), needle) {
					continue
				}
				// Build a 200-char snippet around the match
				lower := strings.ToLower(content)
				idx := strings.Index(lower, needle)
				start := idx - 80
				if start < 0 {
					start = 0
				}
				end := idx + len(needle) + 80
				if end > len(content) {
					end = len(content)
				}
				snippet := content[start:end]
				if start > 0 {
					snippet = "…" + snippet
				}
				if end < len(content) {
					snippet = snippet + "…"
				}

				matches = append(matches, match{
					SessionKey: key,
					Index:      i,
					Role:       msg.Role,
					Snippet:    snippet,
				})
				if len(matches) >= input.Limit {
					goto done
				}
			}
		}
	done:
		output := map[string]interface{}{
			"agent_id": input.AgentID,
			"query":    input.Contains,
			"count":    len(matches),
			"matches":  matches,
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: mustJSON(output)}},
		}, nil
	}
}

// readLogsHandler returns gateway log lines with grep/tail/head filtering
// and automatic truncation of long field values.
func readLogsHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			Tail     int    `json:"tail"`
			Head     int    `json:"head"`
			Offset   int    `json:"offset"`
			Contains string `json:"contains"`
			Level    string `json:"level"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &input); err != nil {
			return errorResult("invalid input: " + err.Error()), nil
		}

		if d.LogBuf == nil {
			return errorResult("log buffer not available — dev-mcp must be enabled before any logs are emitted"), nil
		}

		// Fetch all captured lines (newest first from List)
		// Reverse to chronological order for head/tail semantics
		newest := d.LogBuf.List(0)
		lines := make([]string, len(newest))
		for i := range newest {
			lines[i] = newest[len(newest)-1-i].Text // oldest first
		}

		// Apply offset
		if input.Offset > 0 && input.Offset < len(lines) {
			lines = lines[input.Offset:]
		}

		// Apply level filter
		if input.Level != "" {
			lvl := strings.ToUpper(input.Level)
			filtered := lines[:0]
			for _, l := range lines {
				if strings.Contains(l, " "+lvl+" ") || strings.Contains(l, "\x1b[") && strings.Contains(strings.ToUpper(l), lvl) {
					filtered = append(filtered, l)
				}
			}
			lines = filtered
		}

		// Apply contains filter (grep)
		if input.Contains != "" {
			needle := strings.ToLower(input.Contains)
			filtered := lines[:0]
			for _, l := range lines {
				if strings.Contains(strings.ToLower(l), needle) {
					filtered = append(filtered, l)
				}
			}
			lines = filtered
		}

		// Apply head / tail
		switch {
		case input.Tail > 0 && input.Tail < len(lines):
			lines = lines[len(lines)-input.Tail:]
		case input.Head > 0 && input.Head < len(lines):
			lines = lines[:input.Head]
		case input.Tail == 0 && input.Head == 0 && input.Contains == "" && input.Level == "":
			// default: last 50 lines
			if len(lines) > 50 {
				lines = lines[len(lines)-50:]
			}
		}

		// Truncate long field values (e.g. result_b64=QmFsYW5jZXM...)
		truncated := make([]string, len(lines))
		for i, l := range lines {
			truncated[i] = truncateLongValues(l)
		}

		output := map[string]interface{}{
			"total_captured": d.LogBuf.Len(),
			"returned":       len(truncated),
			"lines":          truncated,
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: mustJSON(output)}},
		}, nil
	}
}

// truncateLongValues finds key=VALUE pairs where VALUE is longer than 20 chars
// and truncates to 8 chars + "..." to keep log lines readable.
// Handles both plain and ANSI-escaped log output.
func truncateLongValues(line string) string {
	const maxLen = 20
	const keepLen = 8
	// Walk the string looking for "=" followed by a run of non-space, non-"" chars
	var out strings.Builder
	i := 0
	for i < len(line) {
		eqIdx := strings.IndexByte(line[i:], '=')
		if eqIdx < 0 {
			out.WriteString(line[i:])
			break
		}
		eqIdx += i
		out.WriteString(line[i : eqIdx+1]) // write up to and including "="
		i = eqIdx + 1
		if i >= len(line) {
			break
		}
		// Find end of the value token (space or end of string)
		end := i
		for end < len(line) && line[end] != ' ' && line[end] != '\n' && line[end] != '\t' {
			end++
		}
		val := line[i:end]
		if len(val) > maxLen {
			// Strip ANSI codes for length check but keep them in prefix
			plain := stripANSI(val)
			if len(plain) > maxLen {
				val = plain[:keepLen] + "..."
			}
		}
		out.WriteString(val)
		i = end
	}
	return out.String()
}

// stripANSI removes ANSI escape sequences from s.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip 'm'
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

// readConfigHandler returns the full redacted configuration.
func readConfigHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		configJSON, err := redactConfig(d.Cfg)
		if err != nil {
			return errorResult("failed to redact config: " + err.Error()), nil
		}

		result := &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: configJSON,
				},
			},
		}

		return result, nil
	}
}

// Helper functions

// mustJSON marshals v to JSON, panicking on error (for development).
func mustJSON(v interface{}) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		logger.ErrorCF("devmcp", "JSON marshal failed", map[string]any{"error": err.Error()})
		return `{"error":"failed to marshal JSON"}`
	}
	return string(b)
}

// errorResult creates a tool error result.
func errorResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{
				Text: fmt.Sprintf(`{"error":"%s"}`, msg),
			},
		},
		IsError: true,
	}
}
