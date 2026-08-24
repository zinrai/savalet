package main

import (
	"io"
	"log/slog"
	"strings"
)

// Accepted parameter values are enum members and need no cap. Rejected
// values, unknown names, and request ids are arbitrary client input and
// must not grow logs and error messages without bound.
const maxLoggedClientBytes = 128

func newAuditLogger(w io.Writer) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.TimeKey:
				a.Key = "ts"
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			case slog.MessageKey:
				a.Key = "event"
			case slog.LevelKey:
				a.Value = slog.StringValue(strings.ToLower(a.Value.String()))
			}
			return a
		},
	})
	return slog.New(h)
}

func capString(s string) string {
	if len(s) > maxLoggedClientBytes {
		return s[:maxLoggedClientBytes]
	}
	return s
}

func capParams(values map[string]string) map[string]string {
	capped := make(map[string]string, len(values))
	for k, v := range values {
		capped[capString(k)] = capString(v)
	}
	return capped
}
