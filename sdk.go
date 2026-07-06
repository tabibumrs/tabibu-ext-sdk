// Package sdk is the Tabibu Extension SDK. Import it in your extension's
// main package and call Run with your Extension implementation.
//
// The SDK handles:
//   - .env file loading on startup (dev convenience)
//   - API key → JWT exchange on startup (and background refresh)
//   - Pine HTTP server on EXT_PORT (default 9000)
//   - Static UI serving from ui/dist/ in production (when TABIBU_DEV != "true")
//   - Broker subscription (RabbitMQ or Kafka) when BROKER_URL is set
//   - Graceful drain on SIGTERM → POST /v1/admin/extensions/:name/drain
package sdk

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/BryanMwangi/pine"
	"github.com/joho/godotenv"
	"github.com/Nexus-Labs-254/tabibu-ext-sdk/internal"
)

// Extension is the interface every Tabibu extension must implement.
type Extension interface {
	// OnStart is called once after the SDK is ready. Register HTTP routes
	// on the provided Server. Return a non-nil error to abort startup.
	OnStart(ctx context.Context, server Server) error

	// OnEvent is called for each broker message the extension subscribes to.
	// Return a non-nil error to nack / retry the message.
	OnEvent(ctx context.Context, event Event) error

	// OnShutdown is called when Tabibu sends a drain signal (SIGTERM).
	// Finish in-flight work and return. The SDK will then POST drain-complete
	// and exit 0. The container is force-killed after stop_grace_period seconds.
	OnShutdown(ctx context.Context) error

	// OnConfigUpdate is called when the extension's config is updated in the
	// Tabibu admin panel. Apply new values to running state.
	OnConfigUpdate(ctx context.Context, cfg Config) error
}

// Server is the subset of Pine's API exposed to OnStart.
type Server interface {
	Get(path string, h HandlerFunc)
	Post(path string, h HandlerFunc)
	Put(path string, h HandlerFunc)
	Delete(path string, h HandlerFunc)
}

// Ctx is the request context passed to handler functions.
type Ctx interface {
	Status(code int) Ctx
	JSON(v any) error
	BindJSON(v any) error
	Params(key string) string
	Query(key string) string
	Context() context.Context
	Header(key string) string
}

// HandlerFunc is the signature for HTTP handlers registered via Server.
type HandlerFunc func(Ctx) error

// Event carries a single broker message dispatched to the extension.
type Event struct {
	Name       string          `json:"name"`
	Version    string          `json:"version"`
	OccurredAt time.Time       `json:"occurred_at"`
	Payload    json.RawMessage `json:"payload"` // unmarshal into a typed struct
	ID         string          `json:"id"`      // use for idempotency
}

// Config is a map of key-value pairs from the extension's config in the
// Tabibu admin panel. Keys match the manifest.toml [extension.config] entries.
type Config map[string]string

// Log is the SDK's structured logger. It writes JSON to logs/extension.log and
// stderr. Available after Run() has started. Extension code may use it directly.
var Log *Logger

// Run starts the extension. It:
//  1. Loads .env if present (dev convenience; existing env vars are not overridden)
//  2. Reads env vars (EXT_NAME, TABIBU_URL, TABIBU_API_KEY, EXT_PORT, BROKER_URL, …)
//  3. Exchanges TABIBU_API_KEY for a JWT via POST /v1/admin/extensions/:name/token
//  4. Starts a background goroutine to refresh the JWT at 80% of its lifetime
//  5. Calls ext.OnStart(ctx, server)
//  6. If ui/dist/ exists and TABIBU_DEV != "true", registers a static file server
//     on the same Pine app (SPA fallback to index.html)
//  7. Subscribes to broker topics (skipped when BROKER_URL is empty)
//  8. Intercepts SIGTERM → ext.OnShutdown → POST drain → exit 0
func Run(ext Extension) error {
	// Load .env if present; silently ignored when absent.
	// Existing process env always takes precedence (godotenv.Load does not overwrite).
	_ = godotenv.Load()

	name := env("EXT_NAME", "")
	Log = newLogger(name)
	if name == "" {
		return errorf("EXT_NAME is required")
	}

	Log.Info("starting extension", map[string]any{"name": name})
	tabibuURL := strings.TrimRight(env("TABIBU_URL", "http://localhost:8080"), "/")
	apiKey := env("TABIBU_API_KEY", "")
	extPort := env("EXT_PORT", "9000")
	devMode := env("TABIBU_DEV", "") == "true"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build HTTP client (holds the JWT).
	c := newClient(tabibuURL, name, apiKey)

	// Exchange API key for JWT (no-op when apiKey is empty — dev testing without auth).
	if apiKey != "" {
		if err := c.refreshToken(ctx); err != nil {
			return errorf("token exchange failed: %v", err)
		}
		go c.keepAlive(ctx)
	}

	// Build Pine server.
	app := pine.New()
	srv := &pineServer{app: app}

	if err := ext.OnStart(ctx, srv); err != nil {
		return errorf("OnStart: %v", err)
	}

	// Static UI serving in prod (not when Vite dev server is in use).
	if !devMode {
		if _, err := os.Stat("ui/dist"); err == nil {
			registerStaticUI(app)
		}
	}

	// Broker subscription.
	internal.StartBroker(ctx, name, func(bctx context.Context, msg internal.BrokerMessage) error {
		payload, err := internal.MarshalEvent(msg)
		if err != nil {
			return err
		}
		event := Event{
			Name:       msg.Topic,
			OccurredAt: time.Now().UTC(),
			Payload:    json.RawMessage(payload),
			ID:         msg.Headers["x-event-id"],
		}
		return ext.OnEvent(bctx, event)
	})

	// Graceful drain on SIGTERM.
	go internal.WatchSignal(ctx, cancel, name, tabibuURL, apiKey, ext.OnShutdown)

	return app.Start(":" + extPort)
}

// env returns the value of the named env var, falling back to def.
func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
