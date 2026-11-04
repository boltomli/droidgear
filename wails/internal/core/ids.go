package core

import (
	"crypto/rand"
	"errors"
	"fmt"
	"time"
)

// NowRFC3339 is the timestamp format every tool module writes into its
// config files (UTC, second precision). Shared by internal/app and the
// service layer so the GUI and `dg` produce identical values.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ValidateProfileID accepts the same ids as the Rust core: non-empty,
// alphanumeric plus '-' and '_'. Profile namespaces are shared across
// tools, so the rule lives in one place.
func ValidateProfileID(id string) error {
	if id == "" {
		return errors.New("Invalid profile id")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return errors.New("Invalid profile id")
	}
	return nil
}

// NewUUID returns a random (version 4) UUID string, generated with
// crypto/rand — no external dependency needed.
func NewUUID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// crypto/rand never fails on supported platforms; fall back to a
		// zero-ish value rather than panicking.
		return "00000000-0000-4000-8000-000000000000"
	}
	buf[6] = (buf[6] & 0x0f) | 0x40 // version 4
	buf[8] = (buf[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16])
}

// StrPtr returns a pointer to value. Tool modules use pointer fields to
// distinguish "unset" from "empty string" when their JSON round-trips, so
// the helper has to be shared by internal/app and the service layer.
func StrPtr(value string) *string {
	return &value
}
