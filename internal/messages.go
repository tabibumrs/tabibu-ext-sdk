package internal

import "encoding/json"

// Message type constants — mirror of the server's pkg/extension/message.go.
// These are kept in sync manually; no shared code between repos.
const (
	// Received on stdin (runtime → extension)
	MsgEvent        = "event"
	MsgActionInvoke = "action_invoke"
	MsgConfigUpdate = "config_update"
	MsgShutdown     = "shutdown"

	// Sent to stdout (extension → runtime)
	MsgServiceReq = "service_req"
	MsgServiceRes = "service_res"
	MsgActionRes  = "action_res"
	MsgHeartbeat  = "heartbeat"
	MsgDrainDone  = "drain_done"
)

// Message is a single NDJSON frame on the stdio channel.
type Message struct {
	ID   string          `json:"id,omitempty"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ServiceReqPayload is the data field of a service_req message.
type ServiceReqPayload struct {
	Service string          `json:"service"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// ServiceResPayload is the data field of a service_res message.
type ServiceResPayload struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// EventPayload is the data field of an event message.
type EventPayload struct {
	Name    string          `json:"name"`
	Payload json.RawMessage `json:"payload"`
}

// ShutdownPayload is the data field of a shutdown message.
type ShutdownPayload struct {
	GraceSeconds int `json:"grace_seconds"`
}

// HeartbeatPayload is the data field of a heartbeat message.
type HeartbeatPayload struct {
	PID int `json:"pid"`
}
