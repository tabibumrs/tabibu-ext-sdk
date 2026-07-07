# tabibu-ext-sdk

Go SDK for building Tabibu extensions. Import it in your extension's `main` package, implement the `Extension` interface, and call `sdk.Run`.

The SDK handles:

- `.env` loading on startup (dev convenience)
- API key → JWT exchange on startup and background refresh
- Pine HTTP server on `EXT_PORT`
- Static UI serving from `ui/dist/` in production
- Broker subscription (RabbitMQ or Kafka) for event-driven extensions
- Graceful drain on SIGTERM — calls `OnShutdown`, notifies Tabibu, then exits 0

---

## Requirements

- Go 1.24+
- A running Tabibu server
- For event-driven extensions: a RabbitMQ or Kafka broker (Tabibu's SQLite broker cannot be used from external processes)

---

## Installation

```bash
go get github.com/Nexus-Labs-254/tabibu-ext-sdk
```

---

## Quick start

```go
package main

import (
    "context"
    "log"
    "net/http"

    sdk "github.com/Nexus-Labs-254/tabibu-ext-sdk"
)

type MyExtension struct{}

func (e *MyExtension) OnStart(ctx context.Context, server sdk.Server) error {
    server.Get("/health", func(c sdk.Ctx) error {
        return c.JSON(map[string]string{"status": "ok"})
    })
    return nil
}

func (e *MyExtension) OnEvent(ctx context.Context, event sdk.Event) error {
    sdk.Log.Info("received event", map[string]any{"name": event.Name})
    return nil
}

func (e *MyExtension) OnShutdown(ctx context.Context) error { return nil }

func (e *MyExtension) OnConfigUpdate(ctx context.Context, cfg sdk.Config) error { return nil }

func main() {
    if err := sdk.Run(&MyExtension{}); err != nil {
        log.Fatal(err)
    }
}
```

See the [hello-world example](examples/hello-world/) for a fuller walkthrough.

---

## The `Extension` interface

```go
type Extension interface {
    // Called once after the SDK is ready. Register HTTP routes here.
    // Return a non-nil error to abort startup.
    OnStart(ctx context.Context, server Server) error

    // Called for each broker message matching EXT_SUBSCRIBE_EVENTS.
    // Return non-nil to nack / retry the message.
    OnEvent(ctx context.Context, event Event) error

    // Called on SIGTERM. Finish in-flight work and return.
    // The SDK posts drain-complete to Tabibu and exits 0.
    OnShutdown(ctx context.Context) error

    // Called when the extension's config is updated in the Tabibu admin panel.
    OnConfigUpdate(ctx context.Context, cfg Config) error
}
```

---

## Registering HTTP routes

Routes are registered on the `Server` interface provided to `OnStart`. The server is a thin wrapper over [Pine](https://github.com/BryanMwangi/pine).

```go
func (e *MyExtension) OnStart(ctx context.Context, server sdk.Server) error {
    server.Get("/items", e.listItems)
    server.Post("/items", e.createItem)
    server.Put("/items/:id", e.updateItem)
    server.Delete("/items/:id", e.deleteItem)
    return nil
}

func (e *MyExtension) listItems(c sdk.Ctx) error {
    id := c.Params("id")
    q  := c.Query("filter")
    hdr := c.Header("X-Custom")
    return c.JSON(map[string]any{"id": id, "filter": q, "header": hdr})
}

func (e *MyExtension) createItem(c sdk.Ctx) error {
    var body struct{ Name string `json:"name"` }
    if err := c.BindJSON(&body); err != nil {
        return c.Status(http.StatusBadRequest).JSON(map[string]string{"error": "invalid body"})
    }
    return c.Status(http.StatusCreated).JSON(map[string]string{"name": body.Name})
}
```

All routes are served on `EXT_PORT` (default `9000`). Tabibu routes `/v1/ui/:name/*` to them via its reverse proxy.

---

## Handling events

Events arrive when the Tabibu broker publishes a topic that matches `EXT_SUBSCRIBE_EVENTS`. Typed payload structs for known Tabibu events are included in the SDK.

```go
func (e *MyExtension) OnEvent(ctx context.Context, event sdk.Event) error {
    switch event.Name {

    case sdk.EventPaymentRequested:
        var p sdk.PaymentRequestedPayload
        if err := json.Unmarshal(event.Payload, &p); err != nil {
            return err
        }
        return e.handlePayment(ctx, p)

    case sdk.EventBillCancelled:
        var p sdk.BillCancelledPayload
        if err := json.Unmarshal(event.Payload, &p); err != nil {
            return err
        }
        return e.handleCancellation(ctx, p)
    }
    return nil
}
```

Returning a non-nil error causes the message to be nacked (RabbitMQ) or left uncommitted (Kafka) and retried.

> **Note:** events require a network-accessible broker. Tabibu's built-in SQLite broker is in-process only and cannot be reached from extension containers. Configure RabbitMQ or Kafka before subscribing to events.

---

## Calling back to Tabibu

The SDK's HTTP client is pre-authenticated with the JWT obtained at startup and automatically refreshes it. Access it via the `Client` field exposed on `sdk.Run`'s internal state — or build your own requests using the `TABIBU_URL` env var and the `Authorization: Bearer <token>` header that the SDK manages.

A simple pattern is to capture a reference to the client in `OnStart`:

```go
// In practice, expose the client from sdk.Run or build a thin wrapper.
// The SDK guarantees TABIBU_URL and the JWT are set before OnStart is called.
func (e *MyExtension) OnStart(ctx context.Context, server sdk.Server) error {
    tabibuURL := os.Getenv("TABIBU_URL")

    server.Post("/action", func(c sdk.Ctx) error {
        // The SDK's client is internal; build the request manually using the
        // JWT the extension obtained via the standard token exchange.
        req, _ := http.NewRequestWithContext(c.Context(), http.MethodGet,
            tabibuURL+"/v1/billing/bills", nil)
        req.Header.Set("Authorization", "Bearer "+e.token) // set in OnStart after exchange
        resp, err := http.DefaultClient.Do(req)
        // ...
        return c.JSON(map[string]string{"status": "ok"})
    })
    return nil
}
```

Extensions call the same `/v1/` routes the Tabibu mobile app uses. No separate extension-specific API exists.

---

## Config

`OnConfigUpdate` receives a `Config` (a `map[string]string`) whenever the admin updates the extension's configuration in the Tabibu admin panel. Apply values to running state:

```go
type MyExtension struct {
    greeting string
}

func (e *MyExtension) OnConfigUpdate(_ context.Context, cfg sdk.Config) error {
    if v, ok := cfg["greeting"]; ok {
        e.greeting = v
    }
    return nil
}
```

---

## Logging

`sdk.Log` is a structured logger available after `Run()` starts. It writes JSON to both `logs/extension.log` and stderr.

```go
sdk.Log.Info("payment initiated", map[string]any{"bill_id": billID, "amount": amount})
sdk.Log.Warn("retrying request", map[string]any{"attempt": 2})
sdk.Log.Error("mpesa callback failed", map[string]any{"error": err.Error()})
```

---

## Environment variables

| Variable               | Required   | Default                 | Description                                                 |
| ---------------------- | ---------- | ----------------------- | ----------------------------------------------------------- |
| `EXT_NAME`             | yes        | —                       | Must match `name` in `manifest.toml`                        |
| `TABIBU_URL`           | yes        | `http://localhost:8080` | Base URL of the Tabibu server                               |
| `TABIBU_API_KEY`       | yes (prod) | —                       | API key issued by `tabibu extension install`                |
| `EXT_PORT`             | no         | `9000`                  | Port the extension HTTP server listens on                   |
| `BROKER_URL`           | no         | —                       | AMQP or Kafka URL; omit to disable broker                   |
| `BROKER_TYPE`          | no         | inferred                | `rabbitmq` or `kafka` (inferred from URL prefix if omitted) |
| `EXT_SUBSCRIBE_EVENTS` | no         | —                       | Comma-separated event topics to subscribe to                |
| `TABIBU_DEV`           | no         | —                       | Set to `true` in dev — skips static file serving            |

The SDK calls `godotenv.Load()` on startup, so you can put these in a `.env` file during local development. Existing process environment variables always take precedence.

---

## UI extensions

If your extension has a web UI, build it to `ui/dist/` (any framework — `npm run build` is the convention). In production the SDK automatically serves those files from the same HTTP port as the backend, with SPA fallback to `ui/dist/index.html` for client-side routing.

In dev, run the Vite (or equivalent) dev server separately and point Tabibu's proxy at it via the `ui_port` field in `manifest.toml`.

Set `has_ui: true` in `manifest.toml` so the Tabibu app shows the embedded WebView.

---

## `manifest.toml` reference

```toml
[extension]
name             = "my-extension"
version          = "1.0.0"
description      = "What this extension does"
author           = "Your Name <you@example.com>"
category         = "billing"          # billing | clinical | admin | other
min_tabibu       = "1.0.0"
stop_grace_period = 30                # seconds before force-kill on drain

[extension.events]
subscribe = [
    "billing.payment_requested",
    "billing.bill_cancelled",
]

[extension.privileges]
required = "billing.view"             # privilege the extension's JWT will carry

[extension.ui]
has_ui = false
port   = 3100                         # Vite dev server port (dev only)
path   = "/"
```

---

## Dev mode setup

```
# Terminal 1 — Tabibu server, pointing at your local extension directory
go run . server start --ext-dev-dir /path/to/my-extension

# Terminal 2 — extension backend
EXT_NAME=my-extension \
EXT_PORT=9000 \
TABIBU_URL=http://localhost:8080 \
TABIBU_API_KEY=<key-from-install> \
TABIBU_DEV=true \
go run .

# Terminal 3 — UI dev server (only for has_ui = true extensions)
cd ui && npm run dev   # runs on the port configured in manifest.toml
```

Tabibu auto-registers the extension from `manifest.toml` when `--ext-dev-dir` is supplied — no `tabibu extension install` needed in dev.

---

## Production / Docker

A multi-stage Dockerfile for extensions with a UI:

```dockerfile
FROM node:22-alpine AS ui-builder
WORKDIR /ui
COPY ui/package*.json ./
RUN npm ci
COPY ui/ .
RUN npm run build

FROM golang:1.24-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /extension .

FROM gcr.io/distroless/static-debian12
COPY --from=ui-builder /ui/dist ui/dist
COPY --from=go-builder /extension .
ENTRYPOINT ["./extension"]
```

For extensions without a UI, omit the `ui-builder` stage.

Install via the Tabibu CLI after pushing your image:

```bash
tabibu extension install \
  --name my-extension \
  --image ghcr.io/your-org/my-extension:1.0.0 \
  --version 1.0.0 \
  --category billing
```

---

## Built-in event types

| Constant                    | Topic string                | Published when                                   |
| --------------------------- | --------------------------- | ------------------------------------------------ |
| `sdk.EventPaymentRequested` | `billing.payment_requested` | A bill is ready for external collection          |
| `sdk.EventBillCancelled`    | `billing.bill_cancelled`    | A bill is cancelled — abandon in-flight requests |
