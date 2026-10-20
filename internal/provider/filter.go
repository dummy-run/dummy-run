package provider

import (
	"errors"
	"fmt"
	"strings"
)

// ErrContentFiltered marks a call that the model provider refused because its content filter or safety
// system objected to the request or the reply. A page that holds hostile text can cause this, so the
// runner reports it as an outcome of its own and not as a harness failure.
var ErrContentFiltered = errors.New("provider: the model provider's content filter refused the request")

// filtered builds an error that satisfies errors.Is(err, ErrContentFiltered) and says which provider
// and what it reported.
func filtered(kind, detail string, cause error) error {
	if cause == nil {
		return fmt.Errorf("provider: %s content filter: %s (%w)", kind, detail, ErrContentFiltered)
	}
	return fmt.Errorf("provider: %s content filter: %s (%w: %w)", kind, detail, ErrContentFiltered, cause)
}

// filterReasons are the finish reasons and error codes that mean a content filter refused the call.
var filterReasons = map[string]bool{
	"content_filter": true, "contentfilter": true, "responsibleaipolicyviolation": true,
	"refusal": true, "content_filtered": true, "guardrail_intervened": true,
	"safety": true, "prohibited_content": true, "blocklist": true, "spii": true, "image_safety": true,
}

// isFilterReason reports whether a provider's finish reason, error code or block reason is a content
// filter refusal. The incomplete: prefix of a Responses API status is ignored.
func isFilterReason(s string) bool {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "incomplete:")))
	return filterReasons[s]
}

// isFilterAPIError reports whether an HTTP error body is a content filter refusal. Azure OpenAI answers
// 400 with the code content_filter and a message about its content management policy.
func isFilterAPIError(code, message string) bool {
	if isFilterReason(code) {
		return true
	}
	m := strings.ToLower(message)
	return strings.Contains(m, "content management policy") || strings.Contains(m, "responsibleaipolicyviolation")
}
