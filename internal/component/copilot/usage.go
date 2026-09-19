package copilot

import (
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// Usage is optional metadata: missing/invalid output must not suppress a reply.
// Read only the unique invocation's file; never scrape a session/auth directory.
func readUsage(path string) coremodel.MessageUsage {
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() > 1024*1024 {
		return coremodel.MessageUsage{}
	}
	f, err := os.Open(path)
	if err != nil {
		return coremodel.MessageUsage{}
	}
	defer f.Close()
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) {
		return coremodel.MessageUsage{}
	}
	body, err := io.ReadAll(io.LimitReader(f, 1024*1024+1))
	if err != nil || len(body) > 1024*1024 {
		return coremodel.MessageUsage{}
	}
	return parseUsage(body)
}
func parseUsage(body []byte) coremodel.MessageUsage {
	var report struct {
		Models map[string]struct {
			Usage struct {
				Input  *int64 `json:"inputTokens"`
				Read   *int64 `json:"cacheReadTokens"`
				Write  *int64 `json:"cacheWriteTokens"`
				Output *int64 `json:"outputTokens"`
			} `json:"usage"`
		} `json:"modelMetrics"`
	}
	if json.Unmarshal(body, &report) != nil || len(report.Models) == 0 || len(report.Models) > 64 {
		return coremodel.MessageUsage{}
	}
	var inputs, reads, writes, outputs []*int64
	var names []string
	for name, m := range report.Models {
		if len(name) > 128 || strings.ContainsAny(name, "\r\n\x00") {
			return coremodel.MessageUsage{}
		}
		names = append(names, name)
		inputs = append(inputs, m.Usage.Input)
		reads = append(reads, m.Usage.Read)
		writes = append(writes, m.Usage.Write)
		outputs = append(outputs, m.Usage.Output)
	}
	sort.Strings(names)
	return coremodel.MessageUsage{Model: strings.Join(names, ", "), Scope: "provider usage snapshot at invocation end (not a last-turn delta)", InputTokens: agentcommon.SumUsage(inputs...), CachedInputTokens: agentcommon.SumUsage(reads...), CacheWriteTokens: agentcommon.SumUsage(writes...), OutputTokens: agentcommon.SumUsage(outputs...)}
}
