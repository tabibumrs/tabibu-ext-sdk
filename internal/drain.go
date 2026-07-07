package internal

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// WatchSignal is the SIGTERM/SIGINT fallback drain path. The primary shutdown
// path is a "shutdown" message arriving on stdin (handled by sdk.go). This
// goroutine only fires when the process receives a signal directly — e.g.
// when the OS kills the process before the Extension Runtime can send a
// graceful shutdown message.
//
// Run in a goroutine from sdk.Run.
func WatchSignal(ctx context.Context, cancel context.CancelFunc, onShutdown func(context.Context) error, conn *Conn) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(quit)

	select {
	case <-ctx.Done():
		return
	case sig := <-quit:
		log.Printf("received %s — starting graceful drain", sig)
	}

	Drain(ctx, cancel, onShutdown, conn)
}

// Drain runs the graceful shutdown sequence:
//  1. Calls onShutdown (with a 30 s timeout)
//  2. Writes {"type":"drain_done"} to stdout so the Extension Runtime knows
//     the process is safe to terminate
//  3. Cancels the context to tear down all background goroutines
func Drain(ctx context.Context, cancel context.CancelFunc, onShutdown func(context.Context) error, conn *Conn) {
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := onShutdown(shutdownCtx); err != nil {
		log.Printf("OnShutdown error: %v", err)
	}

	data, _ := json.Marshal(struct{}{})
	if err := conn.Send(Message{Type: MsgDrainDone, Data: json.RawMessage(data)}); err != nil {
		log.Printf("drain_done send error: %v", err)
	}

	cancel()
	log.Println("drain complete — exiting")
}
