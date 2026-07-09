package outlook

import "strings"

func (c *Component) parseMessage(m graphMessage) storedMessage {
	from := m.From.EmailAddress
	if strings.TrimSpace(from.Address) == "" {
		from = m.Sender.EmailAddress
	}
	return storedMessage{OutlookMessageID: strings.TrimSpace(m.ID), ConversationID: strings.TrimSpace(m.ConversationID), InternetMessageID: strings.TrimSpace(m.InternetMessageID), MailboxEmail: c.providerChannelID(), FromEmail: strings.ToLower(strings.TrimSpace(from.Address)), FromLabel: strings.TrimSpace(firstNonEmpty(from.Name, from.Address)), Subject: strings.TrimSpace(m.Subject), ReceivedAt: m.ReceivedDateTime, BodyPreview: strings.TrimSpace(m.BodyPreview), BodyText: bodyText(m)}
}
func bodyText(m graphMessage) string {
	if strings.EqualFold(m.Body.ContentType, "text") {
		return strings.TrimSpace(m.Body.Content)
	}
	if strings.TrimSpace(m.BodyPreview) != "" {
		return strings.TrimSpace(m.BodyPreview)
	}
	return strings.TrimSpace(m.Body.Content)
}
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
