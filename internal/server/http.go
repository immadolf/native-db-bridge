package server

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/tools"
)

// defaultMCPPath is the default HTTP path for the MCP endpoint.
const defaultMCPPath = "/mcp"

// Server is the Streamable HTTP MCP server. It listens on a local address,
// authenticates requests via Bearer token, and dispatches JSON-RPC 2.0
// tool calls to the appropriate handler.
type Server struct {
	cfg        config.ServerConfig
	token      string
	mcpPath    string
	dispatcher map[string]toolHandler
	httpServer *http.Server
}

// NewServer creates a new Server that binds to the configured address
// (must be 127.0.0.1) and routes MCP requests through the tool handlers.
func NewServer(cfg config.ServerConfig, handlers *tools.Handlers) *Server {
	mcpPath := cfg.MCPPath
	if mcpPath == "" {
		mcpPath = defaultMCPPath
	}
	// Ensure path starts with /
	if !strings.HasPrefix(mcpPath, "/") {
		mcpPath = "/" + mcpPath
	}

	s := &Server{
		cfg:        cfg,
		token:      cfg.ClientToken,
		mcpPath:    mcpPath,
		dispatcher: buildDispatcher(handlers),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc(mcpPath, s.handleAuthenticatedMCP)

	s.httpServer = &http.Server{
		Addr:              cfg.Listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s
}

// ListenAndServe starts the HTTP server. It blocks until the server is
// shut down or an error occurs. The address must resolve to 127.0.0.1.
func (s *Server) ListenAndServe() error {
	if err := validateLocalAddress(s.cfg.Listen); err != nil {
		return err
	}
	return s.httpServer.ListenAndServe()
}

// Close gracefully shuts down the HTTP server.
func (s *Server) Close() error {
	return s.httpServer.Close()
}

// handleHealthz responds to /healthz with a simple JSON status.
func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// handleAuthenticatedMCP checks Bearer token auth before forwarding to handleMCP.
func (s *Server) handleAuthenticatedMCP(w http.ResponseWriter, r *http.Request) {
	if !Authorized(r, s.token) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "unauthorized: missing or invalid Bearer token",
		})
		return
	}
	s.handleMCP(w, r)
}

// validateLocalAddress ensures the listen address resolves to 127.0.0.1.
func validateLocalAddress(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if host == "" || host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return nil
	}
	return &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Err: &localAddrError{addr: host},
	}
}

// localAddrError is returned when the listen address is not local.
type localAddrError struct {
	addr string
}

func (e *localAddrError) Error() string {
	return "server must bind to 127.0.0.1, localhost, or ::1; got " + e.addr
}
