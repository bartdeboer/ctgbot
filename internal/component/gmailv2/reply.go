package gmailv2

import (
	"bytes"
	"fmt"
	"mime"
	"net/mail"
	"regexp"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
)

const maxReferencesHeaderBytes = 900

type replySourceHeaders struct {
	MessageID  string
	References string
	Subject    string
	From       string
	ReplyTo    string
	To         string
	Cc         string
}

type replyBuildInput struct {
	Source       replySourceHeaders
	ThreadID     string
	Mode         string
	OverrideTo   []string
	OverrideCc   []string
	Subject      string
	Body         string
	ContentType  string
	Attachments  []message.Media
	SelfAccounts []string
}

func parseReplySourceHeaders(raw []byte) (replySourceHeaders, error) {
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return replySourceHeaders{}, fmt.Errorf("parse original raw message headers: %w", err)
	}
	headers := replySourceHeaders{
		MessageID:  normalizeMessageID(message.Header.Get("Message-ID")),
		References: strings.TrimSpace(message.Header.Get("References")),
		Subject:    decodeHeaderText(message.Header.Get("Subject")),
		From:       strings.TrimSpace(message.Header.Get("From")),
		ReplyTo:    strings.TrimSpace(message.Header.Get("Reply-To")),
		To:         strings.TrimSpace(message.Header.Get("To")),
		Cc:         strings.TrimSpace(message.Header.Get("Cc")),
	}
	if headers.MessageID == "" {
		return replySourceHeaders{}, fmt.Errorf("original gmail message is missing Message-ID; cannot build RFC reply headers")
	}
	return headers, nil
}

func buildReplySendRequest(input replyBuildInput) (component.MessageSendRequest, error) {
	messageID := normalizeMessageID(input.Source.MessageID)
	if messageID == "" {
		return component.MessageSendRequest{}, fmt.Errorf("original gmail message is missing Message-ID; cannot build RFC reply headers")
	}
	to, cc, err := replyRecipients(input)
	if err != nil {
		return component.MessageSendRequest{}, err
	}
	if len(to) == 0 {
		return component.MessageSendRequest{}, fmt.Errorf("reply has no recipients after excluding current account")
	}
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		subject = replySubject(input.Source.Subject)
	}
	return component.MessageSendRequest{
		To:          append([]string(nil), to...),
		Cc:          append([]string(nil), cc...),
		Subject:     subject,
		Body:        input.Body,
		ContentType: strings.TrimSpace(input.ContentType),
		Attachments: append([]message.Media(nil), input.Attachments...),
		ThreadID:    strings.TrimSpace(input.ThreadID),
		InReplyTo:   messageID,
		References:  buildReferences(input.Source.References, messageID),
	}, nil
}

func replyRecipients(input replyBuildInput) ([]string, []string, error) {
	mode := strings.TrimSpace(input.Mode)
	if mode == "" {
		mode = "reply"
	}
	if !strings.EqualFold(mode, "reply") && !strings.EqualFold(mode, "reply-all") {
		return nil, nil, fmt.Errorf("invalid --mode %q: expected reply or reply-all", input.Mode)
	}
	if len(input.OverrideTo) > 0 || len(input.OverrideCc) > 0 {
		to, err := cleanAddressList(input.OverrideTo)
		if err != nil {
			return nil, nil, fmt.Errorf("to: %w", err)
		}
		cc, err := cleanAddressList(input.OverrideCc)
		if err != nil {
			return nil, nil, fmt.Errorf("cc: %w", err)
		}
		return uniqueAddresses(to, nil), uniqueAddresses(cc, nil), nil
	}
	primary := firstNonEmpty(input.Source.ReplyTo, input.Source.From)
	to, err := cleanAddressList([]string{primary})
	if err != nil {
		return nil, nil, fmt.Errorf("reply recipient: %w", err)
	}
	if strings.EqualFold(mode, "reply-all") {
		allTo, err := cleanAddressList([]string{input.Source.To})
		if err != nil {
			return nil, nil, fmt.Errorf("reply-all to: %w", err)
		}
		allCc, err := cleanAddressList([]string{input.Source.Cc})
		if err != nil {
			return nil, nil, fmt.Errorf("reply-all cc: %w", err)
		}
		self := emailSet(input.SelfAccounts)
		to = uniqueAddresses(append(to, allTo...), self)
		ccExclude := emailSet(input.SelfAccounts)
		for _, value := range to {
			if addr, err := mail.ParseAddress(value); err == nil && addr != nil {
				ccExclude[strings.ToLower(strings.TrimSpace(addr.Address))] = true
			}
		}
		return to, uniqueAddresses(allCc, ccExclude), nil
	}
	return uniqueAddresses(to, emailSet(input.SelfAccounts)), nil, nil
}

func replySubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return "Re:"
	}
	if hasReplyPrefix(subject) {
		return subject
	}
	return "Re: " + subject
}

func hasReplyPrefix(subject string) bool {
	return regexp.MustCompile(`(?i)^\s*re\s*:`).MatchString(subject)
}

func buildReferences(previous string, messageID string) string {
	ids := referenceMessageIDs(previous)
	messageID = normalizeMessageID(messageID)
	if messageID != "" && !containsFold(ids, messageID) {
		ids = append(ids, messageID)
	}
	for len(strings.Join(ids, " ")) > maxReferencesHeaderBytes && len(ids) > 1 {
		ids = ids[1:]
	}
	return strings.Join(ids, " ")
}

func referenceMessageIDs(value string) []string {
	fields := strings.Fields(strings.TrimSpace(value))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		id := normalizeMessageID(field)
		if id != "" && !containsFold(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func normalizeMessageID(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), "")
	value = strings.Trim(value, "<>")
	if value == "" {
		return ""
	}
	return "<" + value + ">"
}

func uniqueAddresses(values []string, exclude map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		addr, err := mail.ParseAddress(value)
		key := strings.ToLower(strings.TrimSpace(value))
		if err == nil && addr != nil {
			key = strings.ToLower(strings.TrimSpace(addr.Address))
		}
		if key == "" || seen[key] || exclude[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func emailSet(values []string) map[string]bool {
	out := map[string]bool{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			addr, err := mail.ParseAddress(strings.TrimSpace(part))
			if err == nil && addr != nil {
				out[strings.ToLower(strings.TrimSpace(addr.Address))] = true
				continue
			}
			if part = strings.ToLower(strings.TrimSpace(part)); part != "" && part != strings.ToLower(DefaultUserID) {
				out[part] = true
			}
		}
	}
	return out
}

func containsFold(values []string, needle string) bool {
	for _, value := range values {
		if strings.EqualFold(value, needle) {
			return true
		}
	}
	return false
}

func decodeHeaderText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(value)
	if err != nil {
		return value
	}
	return strings.TrimSpace(decoded)
}
