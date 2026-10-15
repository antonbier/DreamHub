package compose

import (
	"fmt"
	"strings"
)

// Issue is one reason a compose file is refused (or one note about
// something that was understood but is worth knowing). Code is stable and
// translatable (the UI looks it up as apps.compose.issue.<code>); Message
// is the English fallback, built from the same parts.
type Issue struct {
	Code    string `json:"code"`
	Service string `json:"service,omitempty"`
	Key     string `json:"key,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Message string `json:"message"`
}

func (i Issue) Error() string { return i.Message }

// Issues is a list of Issue that is also an error, so a refusal can carry
// every reason at once instead of one per attempt.
type Issues []Issue

func (is Issues) Error() string {
	parts := make([]string, len(is))
	for i, x := range is {
		parts[i] = x.Message
	}
	return strings.Join(parts, "; ")
}

// issue builds an Issue; the message names the service and key when set.
func issue(code, service, key, detail, format string, args ...any) Issue {
	return Issue{Code: code, Service: service, Key: key, Detail: detail, Message: fmt.Sprintf(format, args...)}
}
