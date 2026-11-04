package channels

import "errors"

// ErrUpstream marks a failure talking to a provider endpoint: DNS and
// timeout errors, non-2xx responses, rejected logins. The GUI renders the
// wrapped message verbatim (string unchanged, so error-i18n keeps working),
// while `dg` maps it to exit code 4 — that distinction is the whole point of
// the sentinel (design doc §2: the service owns error classification).
var ErrUpstream = errors.New("upstream provider request failed")

// upstreamError keeps the original message and adds the ErrUpstream mark.
type upstreamError struct{ err error }

func (e upstreamError) Error() string { return e.err.Error() }

func (e upstreamError) Unwrap() error { return e.err }

func (e upstreamError) Is(target error) bool { return target == ErrUpstream }

// upstream marks err as an upstream failure without changing its text.
// A nil error stays nil.
func upstream(err error) error {
	if err == nil {
		return nil
	}
	return upstreamError{err: err}
}
