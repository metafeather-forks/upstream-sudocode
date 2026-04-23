package mcpserver

import (
	"net/http"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	sseServer *server.SSEServer
	once      sync.Once
)

func getSSEServer() *server.SSEServer {
	once.Do(func() {
		s := server.NewMCPServer("sudocode", "1.0.0",
			server.WithToolCapabilities(true),
		)
		RegisterTools(s)

		sseServer = server.NewSSEServer(s,
			server.WithStaticBasePath("/api/mcp"),
			server.WithSSEEndpoint("/sse"),
			server.WithMessageEndpoint("/message"),
		)
	})
	return sseServer
}

// NewMCPServer creates a configured MCP server with all sudocode tools registered.
// Exported for use by the stdio shim and tests.
func NewMCPServer() *server.MCPServer {
	s := server.NewMCPServer("sudocode", "1.0.0",
		server.WithToolCapabilities(true),
	)
	RegisterTools(s)
	return s
}

// MCPTools returns the list of MCP tool definitions (for testing).
func MCPTools() []mcp.Tool {
	return []mcp.Tool{
		readyTool(),
		listIssuesTool(),
		showIssueTool(),
		upsertIssueTool(),
		listSpecsTool(),
		showSpecTool(),
		upsertSpecTool(),
		linkTool(),
		addReferenceTool(),
		addFeedbackTool(),
		getProjectIDTool(),
	}
}

// MCP handles MCP protocol requests over HTTP/SSE.
//
//encore:api public raw path=/api/mcp/*wildcard
func MCP(w http.ResponseWriter, r *http.Request) {
	getSSEServer().ServeHTTP(w, r)
}
