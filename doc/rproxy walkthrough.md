# Walkthrough: Implementation of `internal/rproxy`

Created the new package `internal/rproxy` providing an HTTP reverse proxy using Go's standard library `httputil.ReverseProxy`. No existing files were modified.

## What Was Added

### 1. [internal/rproxy/rproxy.go](file:///d:/DevFiles/VSCode/source/repos/wol-proxy/internal/rproxy/rproxy.go)
* **API Compatibility:** Implements `NewServer(cfg *config.Config) *Server` and `Start(ctx context.Context, wg *sync.WaitGroup) error`, matching the existing `proxy.Server` interface.
* **HTTP Reverse Proxy (`httputil.ReverseProxy`):**
  * Target: `http://<cfg.ServerIP>:<cfg.ProxyPort>`.
  * Modern `Rewrite` handler (`r.SetURL(targetURL)`, `r.SetXForwarded()`).
  * `FlushInterval: -1` to stream LLM tokens (e.g. Ollama) immediately without proxy buffering.
  * Structured error handler logging via `slog`.
* **WOL Dialing Integration:**
  * Embedded inside `http.Transport.DialContext`.
  * On initial dial failure to backend, triggers `wol.Send()` to wake the remote server, then retries dialing.
* **HTTP Server & Lifecycle:**
  * Uses `http.Server` listening on `:<cfg.ProxyPort>`.
  * Shuts down gracefully via `httpServer.Shutdown()` upon `ctx.Done()`.

### 2. [internal/rproxy/rproxy_test.go](file:///d:/DevFiles/VSCode/source/repos/wol-proxy/internal/rproxy/rproxy_test.go)
* Unit tests verifying `NewServer`, server startup/shutdown lifecycle, and direct WOL dialer connectivity.

---

## Verification Results

### Automated Tests
- `go vet ./...`: Passed with 0 errors.
- `go build ./cmd/wolproxy`: Passed cleanly.
- `go test -v ./internal/rproxy/...`:
  ```text
  === RUN   TestNewServer
  --- PASS: TestNewServer (0.00s)
  === RUN   TestServer_StartAndShutdown
  --- PASS: TestServer_StartAndShutdown (0.50s)
  === RUN   TestDialWithWOL_DirectSuccess
  --- PASS: TestDialWithWOL_DirectSuccess (0.00s)
  PASS
  ok  	github.com/grosenberg/wol-proxy/internal/rproxy	1.108s
  ```
