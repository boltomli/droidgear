package core

import (
	"net/http"
	"time"
)

// NewHTTPClient returns the client used for provider API calls: a plain
// client with a hard timeout. Shared by internal/app (connectivity tester,
// DSH model list) and internal/service/channels so every consumer gets the
// same timeout semantics.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

// TruncateBody caps a response-body snippet for error messages so a
// 8 MB error page never lands in a log line or a CLI stderr.
func TruncateBody(body string) string {
	if len(body) > 500 {
		return body[:500] + "..."
	}
	return body
}
