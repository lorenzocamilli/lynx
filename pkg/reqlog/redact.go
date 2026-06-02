package reqlog

import "net/http"

// redactMask replaces the value of any redacted header.
const redactMask = "[REDACTED]"

// redactHeaders masks the values of the named headers in place. Header names
// are matched case-insensitively (http.Header canonicalizes keys). Headers not
// present are left untouched. The caller must pass a copy it owns — this must
// never mutate headers on a request/response that is still being forwarded.
func redactHeaders(h http.Header, names []string) {
	for _, name := range names {
		if len(h.Values(name)) > 0 {
			h.Set(name, redactMask)
		}
	}
}
