package claude

import "testing"

func TestUsageResultAndModelTotals(t *testing.T) {
	u := parseUsage([]byte(`{"usage":{"input_tokens":2,"cache_read_input_tokens":8,"cache_creation_input_tokens":0,"output_tokens":3}}`))
	if u.InputTokens == nil || *u.InputTokens != 10 || *u.CachedInputTokens != 8 || u.Scope != "invocation, main agent only" {
		t.Fatal(u)
	}
	u = parseUsage([]byte(`{"usage":{"input_tokens":999},"modelUsage":{"m":{"inputTokens":2,"cacheReadInputTokens":8,"cacheCreationInputTokens":1,"outputTokens":4},"n":{"inputTokens":1,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"outputTokens":2}}}`))
	if u.InputTokens == nil || *u.InputTokens != 12 || *u.OutputTokens != 6 || u.Model != "m, n" {
		t.Fatal(u)
	}
	for _, body := range []string{`{}`, `{"usage":{"input_tokens":-1}}`, `{"usage":{"input_tokens":0}}`} {
		u = parseUsage([]byte(body))
		if u.InputTokens != nil {
			t.Fatal("unknown/invalid is not total zero", u)
		}
	}
}
