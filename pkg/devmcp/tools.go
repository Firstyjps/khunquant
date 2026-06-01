package devmcp

import (
	"context"
	"encoding/json"
	"fmt"
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
		Description: "Read conversation history and summary for a session",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"agent_id":{"type":"string","description":"Agent ID (default: 'main')"},
				"session_key":{"type":"string","description":"Session key"}
			},
			"required":["session_key"]
		}`),
	}, readSessionHistoryHandler(d))

	// Tool 7: read_config — no parameters
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

// readSessionHistoryHandler returns conversation history and summary for a session.
func readSessionHistoryHandler(d Deps) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var input struct {
			AgentID    string `json:"agent_id"`
			SessionKey string `json:"session_key"`
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
		agent, ok := registry.GetAgent(input.AgentID)
		if !ok {
			return errorResult("agent not found: " + input.AgentID), nil
		}

		// Get history and summary
		history := agent.Sessions.GetHistory(input.SessionKey)
		summary := agent.Sessions.GetSummary(input.SessionKey)

		// Redact message content
		redactedMessages := make([]map[string]interface{}, 0, len(history))
		for _, msg := range history {
			redactedMsg := map[string]interface{}{
				"role": msg.Role,
			}

			if msg.Content != "" {
				redactedMsg["content"] = redactPayload(msg.Content, d.Cfg)
			}

			if len(msg.ToolCalls) > 0 {
				toolCalls := make([]map[string]interface{}, 0, len(msg.ToolCalls))
				for _, tc := range msg.ToolCalls {
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
				redactedMsg["tool_calls"] = toolCalls
			}

			redactedMessages = append(redactedMessages, redactedMsg)
		}

		output := map[string]interface{}{
			"agent_id":    input.AgentID,
			"session_key": input.SessionKey,
			"message_count": len(redactedMessages),
			"summary":     redactPayload(summary, d.Cfg),
			"history":     redactedMessages,
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
