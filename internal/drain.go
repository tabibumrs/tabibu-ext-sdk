package internal

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// WatchSignal blocks until SIGTERM or SIGINT is received, then:
//  1. Calls shutdown() — the extension's OnShutdown hook
//  2. POSTs /v1/admin/extensions/:name/drain using the API key
//  3. Cancels cancel to tear down all background goroutines
//
// The function is designed to run in a goroutine spawned by sdk.Run.
func WatchSignal(ctx context.Context, cancel context.CancelFunc, extName, tabibuURL, apiKey string, shutdown func(context.Context) error) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(quit)

	select {
	case <-ctx.Done():
		return
	case sig := <-quit:
		log.Printf("received %s — starting graceful drain", sig)
	}

	// Give OnShutdown up to 30 seconds.
	drainCtx, drainCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer drainCancel()

	if err := shutdown(drainCtx); err != nil {
		log.Printf("OnShutdown error: %v", err)
	}

	if err := postDrainComplete(drainCtx, tabibuURL, extName, apiKey); err != nil {
		log.Printf("drain-complete POST failed: %v", err)
	}

	cancel()
	log.Println("drain complete — exiting")
}

func postDrainComplete(ctx context.Context, baseURL, name, apiKey string) error {
	url := fmt.Sprintf("%s/v1/admin/extensions/%s/drain", baseURL, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("drain POST returned status %d", resp.StatusCode)
	}
	return nil
}
