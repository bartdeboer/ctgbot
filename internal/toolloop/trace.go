package toolloop

import (
	"fmt"
	"strings"
)

// DebugFiles names the durable files a caller can inspect after a toolloop run.
type DebugFiles struct {
	Request string
	Result  string
	Events  string
}

func (f DebugFiles) String() string {
	return fmt.Sprintf("toolloop debug files retained:\nrequest: %s\nresult: %s\nevents: %s", f.Request, f.Result, f.Events)
}

func FormatTrace(trace []TraceStep, maxRunes int) string {
	if len(trace) == 0 {
		return ""
	}
	var b strings.Builder
	for _, step := range trace {
		fmt.Fprintf(&b, "iteration=%d finish=%q assistant_chars=%d", step.Iteration, step.FinishReason, step.AssistantContentChars)
		if len(step.ToolCalls) > 0 {
			b.WriteString(" tool_calls=")
			b.WriteString(strings.Join(step.ToolCalls, ","))
		}
		if strings.TrimSpace(step.AssistantPreview) != "" {
			b.WriteString("\nassistant_preview:\n")
			b.WriteString(step.AssistantPreview)
		}
		if strings.TrimSpace(step.ReasoningPreview) != "" {
			b.WriteString("\nreasoning_preview:\n")
			b.WriteString(step.ReasoningPreview)
		}
		for _, result := range step.ToolResults {
			fmt.Fprintf(&b, "\ntool_result name=%s is_error=%t", result.Name, result.IsError)
			if strings.TrimSpace(result.OutputPreview) != "" {
				b.WriteString("\n")
				b.WriteString(result.OutputPreview)
			}
		}
		b.WriteString("\n")
	}
	return TailText(b.String(), maxRunes)
}

func TailText(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return "...<truncated>...\n" + string(runes[len(runes)-maxRunes:])
}
