package service

import "testing"

func TestSanitizeUpstreamMap(t *testing.T) {
	m := map[string]any{
		"id":                 "chatcmpl-1",
		"model":              "real-upstream-model",
		"system_fingerprint": "fp_leak",
		"message":            map[string]any{"model": "real-upstream-model"},
		"error":              map[string]any{"message": "your azure deployment xxx key expired", "param": "p", "code": "some_vendor_code"},
		"usage": map[string]any{
			"prompt_tokens":                    10,
			"usage_semantic":                   "anthropic",
			"billing_usage":                    map[string]any{"source": "claude_messages"},
			"claude_cache_creation_5_m_tokens": 1,
		},
		"nested": []any{map[string]any{"model": "real-upstream-model"}},
	}
	if !sanitizeUpstreamMap(m, "gpt-4o-requested") {
		t.Fatal("expected changes to be reported")
	}
	if m["model"] != "gpt-4o-requested" {
		t.Fatalf("top-level model not rewritten: %v", m["model"])
	}
	if _, ok := m["system_fingerprint"]; ok {
		t.Fatal("system_fingerprint not removed")
	}
	if msg := m["message"].(map[string]any); msg["model"] != "gpt-4o-requested" {
		t.Fatalf("claude message.model not rewritten: %v", msg["model"])
	}
	if errObj := m["error"].(map[string]any); errObj["message"] != HiddenUpstreamErrorMessage {
		t.Fatalf("error message not masked: %v", errObj["message"])
	} else if _, ok := errObj["param"]; ok {
		t.Fatal("error param not removed")
	}
	usage := m["usage"].(map[string]any)
	if _, ok := usage["usage_semantic"]; ok {
		t.Fatal("usage_semantic not removed")
	}
	if _, ok := usage["billing_usage"]; ok {
		t.Fatal("billing_usage not removed")
	}
	if _, ok := usage["claude_cache_creation_5_m_tokens"]; ok {
		t.Fatal("claude_cache token field not removed")
	}
	if usage["prompt_tokens"].(int) != 10 {
		t.Fatal("functional usage field was touched")
	}
	if item := m["nested"].([]any)[0].(map[string]any); item["model"] != "gpt-4o-requested" {
		t.Fatal("model inside array not rewritten")
	}

	// untouched bodies report no change so callers skip re-marshalling
	if sanitizeUpstreamMap(map[string]any{"id": "x"}, "gpt-4o") {
		t.Fatal("unexpected change reported for clean body")
	}
	// model kept as-is when origin unknown (avoids fabrication)
	if sanitizeUpstreamMap(map[string]any{"model": "keep"}, "") {
		t.Fatal("unexpected change without origin model")
	}
}
