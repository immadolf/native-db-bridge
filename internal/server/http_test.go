package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"native-db-bridge-mcp/internal/config"
	"native-db-bridge-mcp/internal/tools"
)

// ---------------------------------------------------------------------------
// Auth tests
// ---------------------------------------------------------------------------

func TestBearerTokenAuth(t *testing.T) {
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if !Authorized(req, "secret") {
		t.Fatalf("expected authorized")
	}
	req.Header.Set("Authorization", "Bearer wrong")
	if Authorized(req, "secret") {
		t.Fatalf("wrong token authorized")
	}
}

func TestBearerTokenAuthMissingHeader(t *testing.T) {
	req := httptest.NewRequest("POST", "/mcp", nil)
	if Authorized(req, "secret") {
		t.Fatalf("expected unauthorized when no header")
	}
}

func TestBearerTokenAuthEmptyToken(t *testing.T) {
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer secret")
	if Authorized(req, "") {
		t.Fatalf("expected unauthorized when expected token is empty")
	}
}

func TestBearerTokenAuthMalformedHeader(t *testing.T) {
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Basic secret")
	if Authorized(req, "secret") {
		t.Fatalf("expected unauthorized for non-Bearer scheme")
	}
}

// ---------------------------------------------------------------------------
// Healthz test
// ---------------------------------------------------------------------------

func TestHealthzEndpoint(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
}

func TestHealthzNoAuthRequired(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/healthz", nil)
	// No Authorization header
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("healthz should not require auth, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// MCP auth rejection
// ---------------------------------------------------------------------------

func TestMCPRequiresAuth(t *testing.T) {
	srv := newTestServer(t)

	body := jsonrpcRequest("datasource_list", nil)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMCPRejectsWrongToken(t *testing.T) {
	srv := newTestServer(t)

	body := jsonrpcRequest("datasource_list", nil)
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// MCP JSON-RPC dispatch
// ---------------------------------------------------------------------------

func TestMCPDatasourceList(t *testing.T) {
	srv := newTestServer(t)

	body := jsonrpcRequest("datasource_list", tools.DatasourceListInput{})
	resp := doAuthenticatedMCP(t, srv, body)

	if resp.Error != nil {
		t.Fatalf("expected success, got error: %s", resp.Error.Message)
	}
	if resp.Result == nil {
		t.Fatalf("expected non-nil result")
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	srv := newTestServer(t)

	body := jsonrpcRequest("nonexistent_tool", nil)
	resp := doAuthenticatedMCP(t, srv, body)

	if resp.Error == nil {
		t.Fatalf("expected error for unknown method")
	}
	if resp.Error.Code != errCodeMethodNotFound {
		t.Fatalf("expected error code %d, got %d", errCodeMethodNotFound, resp.Error.Code)
	}
}

func TestMCPInvalidParams(t *testing.T) {
	srv := newTestServer(t)

	// Send a JSON array as params, which cannot be unmarshaled into the expected struct input type.
	body := jsonrpcRequest("datasource_healthcheck", []string{"bad", "params"})
	resp := doAuthenticatedMCP(t, srv, body)

	if resp.Error == nil {
		t.Fatalf("expected error for invalid params")
	}
	if resp.Error.Code != errCodeInvalidParams {
		t.Fatalf("expected error code %d, got %d", errCodeInvalidParams, resp.Error.Code)
	}
}

func TestMCPInvalidJSONRPC(t *testing.T) {
	srv := newTestServer(t)

	raw := map[string]interface{}{
		"jsonrpc": "1.0",
		"method":  "datasource_list",
		"id":      1,
	}
	body, _ := json.Marshal(raw)
	resp := doAuthenticatedMCP(t, srv, body)

	if resp.Error == nil {
		t.Fatalf("expected error for wrong jsonrpc version")
	}
	if resp.Error.Code != errCodeInvalidRequest {
		t.Fatalf("expected error code %d, got %d", errCodeInvalidRequest, resp.Error.Code)
	}
}

func TestMCPInvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader([]byte("not json")))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for parse error, got %d", rec.Code)
	}
}

func TestMCPRejectsNonPost(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest("GET", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)

	resp := decodeJSONRPCResponse(t, rec.Body.Bytes())
	if resp.Error == nil {
		t.Fatalf("expected error for GET request")
	}
	if resp.Error.Code != errCodeInvalidRequest {
		t.Fatalf("expected error code %d, got %d", errCodeInvalidRequest, resp.Error.Code)
	}
}

func TestMCPResponseHasID(t *testing.T) {
	srv := newTestServer(t)

	body := jsonrpcRequest("datasource_list", tools.DatasourceListInput{})
	resp := doAuthenticatedMCP(t, srv, body)

	if len(resp.ID) == 0 {
		t.Fatalf("expected response to echo the request ID")
	}
}

// ---------------------------------------------------------------------------
// ValidateLocalAddress
// ---------------------------------------------------------------------------

func TestValidateLocalAddress(t *testing.T) {
	cases := []struct {
		addr    string
		wantErr bool
	}{
		{"127.0.0.1:8080", false},
		{"localhost:8080", false},
		{"[::1]:8080", false},
		{":8080", false},
		{"0.0.0.0:8080", true},
		{"192.168.1.1:8080", true},
	}

	for _, tc := range cases {
		err := validateLocalAddress(tc.addr)
		if (err != nil) != tc.wantErr {
			t.Errorf("validateLocalAddress(%q): got err=%v, wantErr=%v", tc.addr, err, tc.wantErr)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newTestServer creates a Server with minimal config and no real backends.
func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.ServerConfig{
		Listen:      "127.0.0.1:0",
		MCPPath:     "/mcp",
		Transport:   "streamable_http",
		ClientToken: "test-token",
	}
	// Create handlers with zero-value deps; this is enough for auth/routing tests.
	handlers := tools.NewHandlers(tools.Deps{})
	return NewServer(cfg, handlers)
}

// jsonrpcRequest builds a JSON-RPC 2.0 request body.
func jsonrpcRequest(method string, params interface{}) []byte {
	paramsRaw := json.RawMessage("null")
	if params != nil {
		b, _ := json.Marshal(params)
		paramsRaw = b
	}
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsRaw,
		ID:      json.RawMessage("1"),
	}
	b, _ := json.Marshal(req)
	return b
}

// doAuthenticatedMCP sends a JSON-RPC request with valid auth and returns the response.
func doAuthenticatedMCP(t *testing.T, srv *Server, body []byte) JSONRPCResponse {
	t.Helper()
	req := httptest.NewRequest("POST", "/mcp", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(rec, req)
	return decodeJSONRPCResponse(t, rec.Body.Bytes())
}

// decodeJSONRPCResponse unmarshals a JSON-RPC 2.0 response.
func decodeJSONRPCResponse(t *testing.T, data []byte) JSONRPCResponse {
	t.Helper()
	var resp JSONRPCResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		t.Fatalf("failed to decode JSON-RPC response: %v\nraw: %s", err, string(data))
	}
	return resp
}
