package outlook

import (
	"strings"
	"time"
)

func (c *Component) inboundPrompt(record storedMessage) string {
	var lines []string
	lines = append(lines, "Outlook message "+record.OutlookMessageID, "From: "+record.FromLabel+emailSuffix(record.FromEmail), "Subject: "+record.Subject)
	if !record.ReceivedAt.IsZero() {
		lines = append(lines, "Date: "+record.ReceivedAt.Format(time.RFC3339))
	}
	lines = append(lines, "", "Body:", "```text", strings.TrimSpace(firstNonEmpty(record.BodyText, record.BodyPreview)), "```")
	return strings.TrimSpace(strings.Join(lines, "\n")) + "\n"
}
func emailSuffix(email string) string {
	if strings.TrimSpace(email) == "" {
		return ""
	}
	return " <" + strings.TrimSpace(email) + ">"
}
