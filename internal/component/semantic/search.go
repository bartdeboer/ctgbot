package semantic

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

type scoredMessage struct {
	ID     string  `json:"id"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
}

type scoreResponse struct {
	Scores []scoredMessage `json:"scores"`
}

func (c *Component) Search(ctx context.Context, req component.SearchRequest) (component.SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return component.SearchResponse{}, fmt.Errorf("missing search query")
	}
	if req.ThreadID.IsNull() {
		return component.SearchResponse{}, fmt.Errorf("missing thread id")
	}
	if c == nil || c.messages == nil {
		return component.SearchResponse{}, fmt.Errorf("missing semantic message source")
	}
	var messages []coremodel.ThreadMessage
	if err := c.messages.ForEachMessage(ctx, component.MessageScope{ThreadID: req.ThreadID, Kinds: semanticMessageKinds()}, func(message coremodel.ThreadMessage) error {
		messages = append(messages, message)
		return nil
	}); err != nil {
		return component.SearchResponse{}, err
	}
	items := searchableMessages(messages)
	maxMessages := req.MaxMessages
	if maxMessages <= 0 {
		maxMessages = c.config.MaxMessages
	}
	if maxMessages > 0 && len(items) > maxMessages {
		items = items[len(items)-maxMessages:]
	}
	if len(items) == 0 {
		return component.SearchResponse{}, nil
	}
	engine, engineRef, err := c.resolveCompletionEngine(ctx)
	if err != nil {
		return component.SearchResponse{}, err
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = strings.TrimSpace(c.config.Model)
	}
	closeSession, err := c.beginInferenceSession(ctx, engine, engineRef, model, req.CompletionIdleTimeout)
	if err != nil {
		return component.SearchResponse{}, err
	}
	defer closeSession()
	limit := req.Limit
	if limit <= 0 {
		limit = c.config.Limit
	}
	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = c.config.BatchSize
	}
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}
	minScore := req.MinScore
	if minScore <= 0 {
		minScore = c.config.MinScore
	}
	if minScore <= 0 {
		minScore = DefaultMinScore
	}
	byID := map[string]coremodel.ThreadMessage{}
	for _, message := range items {
		byID[message.ID.String()] = message
	}
	var results []component.SearchResult
	for start := 0; start < len(items); start += batchSize {
		end := start + batchSize
		if end > len(items) {
			end = len(items)
		}
		batch := items[start:end]
		scores, err := c.scoreBatch(ctx, engine, model, query, batch)
		if err != nil {
			return component.SearchResponse{}, fmt.Errorf("semantic batch %d-%d via %s: %w", start, end, engineRef, err)
		}
		for _, score := range scores {
			if score.Score < minScore {
				continue
			}
			message, ok := byID[strings.TrimSpace(score.ID)]
			if !ok {
				continue
			}
			results = append(results, component.SearchResult{
				MessageID: message.ID,
				ChatID:    message.ChatID,
				ThreadID:  message.ThreadID,
				Excerpt:   excerpt(message.Text, 240),
				Text:      message.Text,
				Score:     clampScore(score.Score),
				Reason:    strings.TrimSpace(score.Reason),
			})
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].MessageID.String() < results[j].MessageID.String()
		}
		return results[i].Score > results[j].Score
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return component.SearchResponse{Results: results}, nil
}

func (c *Component) beginInferenceSession(ctx context.Context, engine component.CompletionEngine, engineRef string, model string, idleTimeout time.Duration) (func(), error) {
	sessionEngine, ok := engine.(component.InferenceSessionEngine)
	if !ok {
		return func() {}, nil
	}
	session, err := sessionEngine.BeginInferenceSession(ctx, component.InferenceSessionOptions{Model: strings.TrimSpace(model), IdleTimeout: idleTimeout})
	if err != nil {
		return nil, fmt.Errorf("begin inference session via %s: %w", engineRef, err)
	}
	return func() {
		if err := session.Close(); err != nil {
			c.log("semantic inference session close failed engine=%s err=%v", engineRef, err)
		}
	}, nil
}

func searchableMessages(messages []coremodel.ThreadMessage) []coremodel.ThreadMessage {
	out := make([]coremodel.ThreadMessage, 0, len(messages))
	for _, message := range messages {
		if !semanticMessage(message) || strings.TrimSpace(message.Text) == "" || message.ID.IsNull() {
			continue
		}
		out = append(out, message)
	}
	return out
}

func semanticMessageKinds() []coremodel.MessageKind {
	return []coremodel.MessageKind{coremodel.MessageKindMessage}
}

func semanticMessage(message coremodel.ThreadMessage) bool {
	if message.Kind != coremodel.MessageKindMessage {
		return false
	}
	switch message.ResolvedRole() {
	case coremodel.MessageRoleUser, coremodel.MessageRoleAgent:
		return true
	default:
		return false
	}
}

func (c *Component) scoreBatch(ctx context.Context, engine component.CompletionEngine, model string, query string, messages []coremodel.ThreadMessage) ([]scoredMessage, error) {
	result, err := engine.Complete(ctx, component.CompletionRequest{
		Model: strings.TrimSpace(model),
		Prompt: component.CompletionPrompt{Messages: []component.CompletionMessage{{
			Role:    component.CompletionRoleUser,
			Content: semanticScoringPrompt(query, messages),
		}}},
		MaxOutputTokens: c.config.MaxOutputTokens,
		ResponseFormat:  "json",
		Reasoning:       component.ReasoningDisabled,
	})
	if err != nil {
		return nil, err
	}
	parsed, err := parseScoreResponse(completionResultText(result))
	if err != nil {
		return nil, err
	}
	return parsed.Scores, nil
}

func semanticScoringPrompt(query string, messages []coremodel.ThreadMessage) string {
	var b strings.Builder
	b.WriteString("You are performing semantic conversation history search.\n\n")
	b.WriteString("Score each message independently for relevance to the query.\n")
	b.WriteString("A message is relevant if it directly discusses the query, discusses a related implementation/detail, or is conceptually associated.\n\n")
	b.WriteString("Return JSON only in this shape:\n")
	b.WriteString(`{"scores":[{"id":"<message_id>","score":0.0,"reason":"short reason"}]}`)
	b.WriteString("\n\nScoring:\n1.0 = strong direct match\n0.7 = related concept\n0.4 = weak association\n0.0 = unrelated\n\n")
	b.WriteString("Query:\n")
	b.WriteString(query)
	b.WriteString("\n\nMessages:\n")
	for _, message := range messages {
		b.WriteString(message.ID.String())
		b.WriteString(" - ")
		b.WriteString(oneLine(message.Text, 1600))
		b.WriteString("\n")
	}
	return b.String()
}

func parseScoreResponse(text string) (scoreResponse, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	var response scoreResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		return scoreResponse{}, err
	}
	return response, nil
}

func clampScore(score float64) float64 {
	switch {
	case score < 0:
		return 0
	case score > 1:
		return 1
	default:
		return score
	}
}

func excerpt(text string, max int) string {
	return oneLine(text, max)
}

func oneLine(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if max <= 0 || utf8.RuneCountInString(text) <= max {
		return text
	}
	runes := []rune(text)
	return string(runes[:max]) + "..."
}
