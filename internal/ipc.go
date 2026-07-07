package internal

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

const callTimeout = 30 * time.Second

// Conn is the extension-side stdio IPC connection.
// It reads NDJSON from stdin and writes NDJSON to stdout.
type Conn struct {
	out    *json.Encoder
	outMu  sync.Mutex
	pending sync.Map   // id → chan Message
	counter atomic.Int64
}

// NewConn creates a Conn that writes to out. Call StartReadLoop to begin
// processing incoming messages from in.
func NewConn(out io.Writer) *Conn {
	return &Conn{out: json.NewEncoder(out)}
}

// StartReadLoop reads NDJSON from in and dispatches each message.
// Service responses (type == MsgServiceRes) are routed to the waiting Call;
// all other messages are forwarded to handler. Runs until in closes or ctx
// is cancelled.
func (c *Conn) StartReadLoop(ctx context.Context, in io.Reader, handler func(Message)) {
	go func() {
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}
			var msg Message
			if err := json.Unmarshal(scanner.Bytes(), &msg); err != nil {
				continue
			}
			// Route correlated service responses back to waiting Call goroutines.
			if msg.ID != "" && msg.Type == MsgServiceRes {
				if ch, ok := c.pending.Load(msg.ID); ok {
					ch.(chan Message) <- msg
					continue
				}
			}
			handler(msg)
		}
	}()
}

// Send writes a single NDJSON message to stdout.
func (c *Conn) Send(msg Message) error {
	c.outMu.Lock()
	defer c.outMu.Unlock()
	return c.out.Encode(msg)
}

// Call sends a message and waits for a correlated service_res response.
// The call fails if no response arrives within callTimeout.
func (c *Conn) Call(ctx context.Context, msg Message) (Message, error) {
	id := fmt.Sprintf("svc-%d", c.counter.Add(1))
	msg.ID = id

	ch := make(chan Message, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	if err := c.Send(msg); err != nil {
		return Message{}, fmt.Errorf("ipc: send: %w", err)
	}

	select {
	case <-ctx.Done():
		return Message{}, ctx.Err()
	case <-time.After(callTimeout):
		return Message{}, fmt.Errorf("ipc: call %s timed out after %s", id, callTimeout)
	case resp := <-ch:
		return resp, nil
	}
}
