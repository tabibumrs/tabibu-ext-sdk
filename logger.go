package sdk

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// Logger writes structured JSON log entries to both logs/extension.log and stderr.
// The log file is opened with O_APPEND so existing content is preserved across
// restarts. In containerised deployments log shipping (e.g. Fluent Bit) reads
// from the file; rotation is handled by the container runtime or logrotate.
type Logger struct {
	out io.Writer
}

func newLogger(_ string) *Logger {
	if err := os.MkdirAll("logs", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "logger: could not create logs/: %v\n", err)
		return &Logger{out: os.Stderr}
	}

	f, err := os.OpenFile("logs/extension.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: could not open logs/extension.log: %v\n", err)
		return &Logger{out: os.Stderr}
	}

	return &Logger{out: io.MultiWriter(f, os.Stderr)}
}

func (lg *Logger) log(level, msg string, fields map[string]any) {
	entry := map[string]any{
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"level": level,
		"msg":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	b, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(lg.out, `{"level":"error","msg":"logger marshal failed","err":%q}`+"\n", err.Error())
		return
	}
	fmt.Fprintf(lg.out, "%s\n", b)
}

func (lg *Logger) Info(msg string, fields ...map[string]any)  { lg.log("info", msg, merge(fields)) }
func (lg *Logger) Warn(msg string, fields ...map[string]any)  { lg.log("warn", msg, merge(fields)) }
func (lg *Logger) Error(msg string, fields ...map[string]any) { lg.log("error", msg, merge(fields)) }

func merge(fields []map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	out := make(map[string]any)
	for _, m := range fields {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
