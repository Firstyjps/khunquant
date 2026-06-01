package devmcp

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/cryptoquantumwave/khunquant/pkg/agent"
	"github.com/cryptoquantumwave/khunquant/pkg/config"
	"github.com/cryptoquantumwave/khunquant/pkg/debugtap"
)

// Deps holds the runtime objects the MCP server reads from.
type Deps struct {
	Loop     *agent.AgentLoop
	DebugTap *debugtap.Store
	LogBuf   *debugtap.LogBuffer
	Cfg      *config.Config
}

// NewMCPServer constructs an mcp.Server with the read-only tool set registered.
// This is the ONLY place mcp.AddTool is called in this package.
func NewMCPServer(d Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "khunquant-dev",
		Version: "1.0.0",
	}, nil)

	registerReadOnlyTools(s, d) // defined in tools.go

	return s
}

// NewHTTPHandler returns an http.Handler that speaks Streamable HTTP MCP.
// Use JSONResponse:true to avoid triggering the shared gateway's 30s WriteTimeout
// (each response is a single application/json body, no hanging SSE stream).
// DNS-rebinding and cross-origin protections are kept ON (SDK defaults).
func NewHTTPHandler(d Deps) http.Handler {
	return mcp.NewStreamableHTTPHandler(
		func(r *http.Request) *mcp.Server { return NewMCPServer(d) },
		&mcp.StreamableHTTPOptions{JSONResponse: true},
	)
}
