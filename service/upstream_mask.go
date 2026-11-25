package service

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/model_setting"

	"github.com/gin-gonic/gin"
)

// HiddenUpstreamErrorMessage replaces real upstream error messages in client
// responses when HideUpstreamInfoEnabled is on.
const HiddenUpstreamErrorMessage = "upstream service error, please try again later or contact the administrator"

// HideUpstreamInfoEnabled reports whether relay responses must not reveal the
// real upstream provider: model/owned_by rewritten to the origin name,
// system_fingerprint and provider-specific fields stripped, upstream headers
// censored and upstream error details replaced.
func HideUpstreamInfoEnabled() bool {
	return model_setting.GetGlobalSettings().HideUpstreamInfoEnabled
}

// upstreamLeakBodyFields is removed from response bodies at any nesting depth
// when hiding is enabled (case-insensitive match on the lowercased key).
var upstreamLeakBodyFields = map[string]struct{}{
	"system_fingerprint":               {},
	"prompt_filter_results":            {},
	"content_filter_results":           {},
	"prompt-filter-results":            {},
	"azure-content-filter-results":     {},
	"usage_semantic":                   {},
	"usage_source":                     {},
	"billing_usage":                    {},
	"claude_cache_creation_5_m_tokens": {},
	"claude_cache_creation_1h_tokens":  {},
}

// SanitizeUpstreamBody cleans an upstream JSON response body before it goes to
// the client. Non-JSON payloads and errors are passed through unchanged.
func SanitizeUpstreamBody(c *gin.Context, data []byte) []byte {
	if !HideUpstreamInfoEnabled() || len(data) == 0 || data[0] != '{' {
		return data
	}
	var obj map[string]any
	if err := common.Unmarshal(data, &obj); err != nil {
		return data
	}
	originModel := ""
	if c != nil {
		originModel = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	if !sanitizeUpstreamMap(obj, originModel) {
		return data
	}
	out, err := common.Marshal(obj)
	if err != nil {
		return data
	}
	return out
}

// sanitizeUpstreamMap rewrites the payload in place; reports whether anything
// changed so callers can skip re-marshalling untouched bodies.
func sanitizeUpstreamMap(m map[string]any, originModel string) bool {
	changed := false
	for k, v := range m {
		if _, leak := upstreamLeakBodyFields[strings.ToLower(k)]; leak {
			delete(m, k)
			changed = true
			continue
		}
		switch tv := v.(type) {
		case map[string]any:
			if k == "error" && sanitizeUpstreamError(tv) {
				changed = true
			}
			if sanitizeUpstreamMap(tv, originModel) {
				changed = true
			}
		case []any:
			for _, item := range tv {
				if im, ok := item.(map[string]any); ok && sanitizeUpstreamMap(im, originModel) {
					changed = true
				}
			}
		}
		if k == "model" && originModel != "" {
			if s, ok := v.(string); ok && s != originModel {
				m[k] = originModel
				changed = true
			}
		}
	}
	return changed
}

// sanitizeUpstreamError masks provider details inside a JSON error object
// (message text, param, metadata); the type code is kept for client handling.
func sanitizeUpstreamError(e map[string]any) bool {
	changed := false
	if _, has := e["message"]; has {
		e["message"] = HiddenUpstreamErrorMessage
		changed = true
	}
	if _, has := e["metadata"]; has {
		delete(e, "metadata")
		changed = true
	}
	if _, has := e["param"]; has {
		delete(e, "param")
		changed = true
	}
	return changed
}
