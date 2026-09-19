package claude

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func parseUsage(data []byte) coremodel.MessageUsage {
	var result struct {
		Models map[string]struct {
			Input      *int64 `json:"inputTokens"`
			CacheRead  *int64 `json:"cacheReadInputTokens"`
			CacheWrite *int64 `json:"cacheCreationInputTokens"`
			Output     *int64 `json:"outputTokens"`
		} `json:"modelUsage"`
		Usage *struct {
			Input      *int64 `json:"input_tokens"`
			CacheRead  *int64 `json:"cache_read_input_tokens"`
			CacheWrite *int64 `json:"cache_creation_input_tokens"`
			Output     *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &result) != nil {
		return coremodel.MessageUsage{}
	}
	var inputs, reads, writes, outputs []*int64
	var names []string
	scope := "invocation, main agent only"
	if len(result.Models) > 0 && len(result.Models) <= 64 {
		scope = "invocation, reported model totals including subagents"
		for name, m := range result.Models {
			if len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
				return coremodel.MessageUsage{}
			}
			names = append(names, name)
			inputs = append(inputs, agentcommon.SumUsage(m.Input, m.CacheRead, m.CacheWrite))
			reads = append(reads, m.CacheRead)
			writes = append(writes, m.CacheWrite)
			outputs = append(outputs, m.Output)
		}
	} else if m := result.Usage; m != nil {
		inputs = append(inputs, agentcommon.SumUsage(m.Input, m.CacheRead, m.CacheWrite))
		reads = append(reads, m.CacheRead)
		writes = append(writes, m.CacheWrite)
		outputs = append(outputs, m.Output)
	} else {
		return coremodel.MessageUsage{}
	}
	sort.Strings(names)
	return coremodel.MessageUsage{Model: strings.Join(names, ", "), Scope: scope, InputTokens: agentcommon.SumUsage(inputs...), CachedInputTokens: agentcommon.SumUsage(reads...), CacheWriteTokens: agentcommon.SumUsage(writes...), OutputTokens: agentcommon.SumUsage(outputs...)}
}
