package guard

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/inbound"
)

type guardResult struct {
	Decision             string   `json:"decision"`
	SpamScore            float64  `json:"spam_score"`
	PersuasionScore      float64  `json:"persuasion_score"`
	ThreatScore          float64  `json:"threat_score"`
	PromptInjectionScore float64  `json:"prompt_injection_score"`
	PhishingScore        float64  `json:"phishing_score"`
	ToolRequestScore     float64  `json:"tool_request_score"`
	Reason               string   `json:"reason"`
	Labels               []string `json:"labels"`
}

func completionResultText(result *component.CompletionResult) string {
	if result == nil || result.Final == nil {
		return ""
	}
	return result.Final.Text
}

func parseGuardResult(text string) (guardResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return guardResult{}, fmt.Errorf("empty guard output")
	}
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	var result guardResult
	if err := decoder.Decode(&result); err != nil {
		return guardResult{}, err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return guardResult{}, fmt.Errorf("guard output contains trailing data")
	}
	result.Decision = strings.ToLower(strings.TrimSpace(result.Decision))
	switch result.Decision {
	case "allow", "quarantine", "deny":
	default:
		return guardResult{}, fmt.Errorf("invalid guard decision %q", result.Decision)
	}
	for name, score := range map[string]float64{
		"spam_score":             result.SpamScore,
		"persuasion_score":       result.PersuasionScore,
		"threat_score":           result.ThreatScore,
		"prompt_injection_score": result.PromptInjectionScore,
		"phishing_score":         result.PhishingScore,
		"tool_request_score":     result.ToolRequestScore,
	} {
		if score < 0 || score > 1 {
			return guardResult{}, fmt.Errorf("%s out of range: %v", name, score)
		}
	}
	return result, nil
}

func (r guardResult) filterResult(input inbound.ChannelEvent, ref string, highRiskScore float64) inbound.FilterResult {
	details := r.logDetails(ref)
	switch r.Decision {
	case "deny":
		return inbound.Drop(input, "guard-deny", details...)
	case "quarantine":
		return inbound.Quarantine(input, "guard-quarantine", details...)
	}
	if r.SpamScore >= highRiskScore ||
		r.PersuasionScore >= highRiskScore ||
		r.ThreatScore >= highRiskScore ||
		r.PromptInjectionScore >= highRiskScore ||
		r.PhishingScore >= highRiskScore ||
		r.ToolRequestScore >= highRiskScore {
		return inbound.Quarantine(input, "guard-quarantine", details...)
	}
	return inbound.Pass(input)
}

func (r guardResult) logDetails(ref string) []string {
	details := []string{
		"guard=" + logValue(ref),
		"guard_decision=" + logValue(r.Decision),
		fmt.Sprintf("scores=spam:%.2f,persuasion:%.2f,threat:%.2f,prompt_injection:%.2f,phishing:%.2f,tool_request:%.2f",
			r.SpamScore,
			r.PersuasionScore,
			r.ThreatScore,
			r.PromptInjectionScore,
			r.PhishingScore,
			r.ToolRequestScore,
		),
	}
	if reason := logValue(r.Reason); reason != "" {
		details = append(details, "guard_reason="+reason)
	}
	if labels := logLabels(r.Labels); labels != "" {
		details = append(details, "guard_labels="+labels)
	}
	return details
}

func logValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func logLabels(labels []string) string {
	values := make([]string, 0, len(labels))
	for _, label := range labels {
		if value := logValue(label); value != "" {
			values = append(values, value)
		}
	}
	return strings.Join(values, ",")
}
