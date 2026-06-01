package indexing

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type SearchRequest struct {
	Query             string
	Mode              string
	IndexingComponent string
	EmbeddingStrategy string
	TitleStrategy     string
	Scope             scope
	Limit             int
}

type SearchResult struct {
	MessageID modeluuid.UUID
	ChatID    modeluuid.UUID
	ThreadID  modeluuid.UUID
	Role      coremodel.MessageRole
	CreatedAt time.Time
	Title     string
	Excerpt   string
	Text      string
	Score     float64
}

func (c *SearchComponent) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, fmt.Errorf("missing search query")
	}
	if req.Limit <= 0 {
		req.Limit = DefaultSearchLimit
	}
	indexingComponent, err := c.resolveIndexingComponent(ctx, req.IndexingComponent)
	if err != nil {
		return nil, err
	}
	switch normalizeSearchMode(req.Mode) {
	case "", searchModeEmbedding:
		return c.searchEmbedding(ctx, indexingComponent, req)
	case searchModeKeyword:
		return c.searchKeyword(ctx, indexingComponent, req)
	default:
		return nil, fmt.Errorf("unsupported search mode: %s", req.Mode)
	}
}

func (c *SearchComponent) resolveIndexingComponent(ctx context.Context, ref string) (*Component, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = DefaultIndexingComponentRef
	}
	if c == nil || c.resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	registration, err := c.resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if loaded == nil || loaded.Component == nil {
		return nil, fmt.Errorf("indexing component not found: %s", registration.Ref())
	}
	indexingComponent, ok := loaded.Component.(*Component)
	if !ok {
		return nil, fmt.Errorf("component %s is %T, not indexing", registration.Ref(), loaded.Component)
	}
	return indexingComponent, nil
}

func (c *SearchComponent) searchEmbedding(ctx context.Context, indexingComponent *Component, req SearchRequest) ([]SearchResult, error) {
	strategy, err := indexingComponent.embeddingSearchStrategy(ctx, req.EmbeddingStrategy)
	if err != nil {
		return nil, err
	}
	embedder, _, err := indexingComponent.resolveEmbeddingEngine(ctx, strategy.ProviderRef)
	if err != nil {
		return nil, err
	}
	response, err := embedder.Embed(ctx, component.EmbeddingRequest{Model: strategy.Model, Inputs: []component.EmbeddingInput{{ID: "query", Text: req.Query, Kind: component.EmbeddingKindQuery}}})
	if err != nil {
		return nil, err
	}
	if len(response.Embeddings) == 0 || len(response.Embeddings[0].Vector) == 0 {
		return nil, fmt.Errorf("embed query returned empty vector")
	}
	queryVector := response.Embeddings[0].Vector
	embeddings, err := indexingComponent.store.searchEmbeddings(ctx, strategy.ID, req.Scope)
	if err != nil {
		return nil, err
	}
	messageIDs := make([]string, 0, len(embeddings))
	for _, embedding := range embeddings {
		messageIDs = append(messageIDs, embedding.MessageID)
	}
	messages, err := c.messagesByID(ctx, req.Scope, messageIDs)
	if err != nil {
		return nil, err
	}
	titles, err := indexingComponent.store.summariesByMessageID(ctx, req.TitleStrategy, messageIDs)
	if err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(embeddings))
	for _, embedding := range embeddings {
		message, ok := messages[embedding.MessageID]
		if !ok {
			continue
		}
		vector := decodeVector(embedding.Embedding)
		if len(vector) != len(queryVector) {
			continue
		}
		results = append(results, resultFromMessage(message, titles[embedding.MessageID], cosineScore(queryVector, vector)))
	}
	sortSearchResults(results)
	return limitSearchResults(results, req.Limit), nil
}

func (c *SearchComponent) searchKeyword(ctx context.Context, indexingComponent *Component, req SearchRequest) ([]SearchResult, error) {
	messages, err := c.messagesForSearchScope(ctx, req.Scope)
	if err != nil {
		return nil, err
	}
	messageIDs := make([]string, 0, len(messages))
	for _, message := range messages {
		messageIDs = append(messageIDs, message.ID.String())
	}
	titles, err := indexingComponent.store.summariesByMessageID(ctx, req.TitleStrategy, messageIDs)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(req.Query))
	results := make([]SearchResult, 0)
	for _, message := range messages {
		text := strings.ToLower(message.Text)
		count := strings.Count(text, query)
		if count == 0 {
			continue
		}
		result := resultFromMessage(message, titles[message.ID.String()], float64(count))
		result.Excerpt = keywordExcerpt(message.Text, req.Query, 240)
		results = append(results, result)
	}
	sortSearchResults(results)
	return limitSearchResults(results, req.Limit), nil
}

func (c *SearchComponent) messagesByID(ctx context.Context, scope scope, ids []string) (map[string]coremodel.ThreadMessage, error) {
	wanted := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	messages, err := c.messagesForSearchScope(ctx, scope)
	if err != nil {
		return nil, err
	}
	out := map[string]coremodel.ThreadMessage{}
	for _, message := range messages {
		id := message.ID.String()
		if _, ok := wanted[id]; ok {
			out[id] = message
		}
	}
	return out, nil
}

func (c *SearchComponent) messagesForSearchScope(ctx context.Context, scope scope) ([]coremodel.ThreadMessage, error) {
	if c == nil || c.messages == nil {
		return nil, fmt.Errorf("missing search message source")
	}
	var messages []coremodel.ThreadMessage
	err := c.messages.ForEachMessage(ctx, component.MessageScope{
		ChatID:   scope.ChatID,
		ThreadID: scope.ThreadID,
		All:      scope.All,
		Limit:    scope.Limit,
		Order:    scope.Order,
		Kinds:    indexableMessageKinds(),
		Roles:    []coremodel.MessageRole{coremodel.MessageRoleUser, coremodel.MessageRoleAgent},
	}, func(message coremodel.ThreadMessage) error {
		if searchableMessage(message) {
			messages = append(messages, message)
		}
		return nil
	})
	return messages, err
}

func (c *Component) embeddingSearchStrategy(ctx context.Context, name string) (*indexStrategy, error) {
	strategy, err := c.store.strategyByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if strategy == nil {
		return nil, fmt.Errorf("embedding strategy not found: %s", normalizeName(name))
	}
	if strategy.Type != StrategyTypeEmbedding {
		return nil, fmt.Errorf("strategy %s is %s, not embedding", strategy.Name, strategy.Type)
	}
	if !strategy.Enabled {
		return nil, fmt.Errorf("embedding strategy disabled: %s", strategy.Name)
	}
	if strings.TrimSpace(strategy.ProviderRef) == "" {
		return nil, fmt.Errorf("embedding strategy %s has no provider", strategy.Name)
	}
	if strings.TrimSpace(strategy.Model) == "" {
		return nil, fmt.Errorf("embedding strategy %s has no model", strategy.Name)
	}
	return strategy, nil
}

func resultFromMessage(message coremodel.ThreadMessage, title string, score float64) SearchResult {
	return SearchResult{
		MessageID: message.ID,
		ChatID:    message.ChatID,
		ThreadID:  message.ThreadID,
		Role:      message.ResolvedRole(),
		CreatedAt: message.CreatedAt,
		Title:     strings.TrimSpace(title),
		Excerpt:   excerpt(message.Text, 240),
		Text:      message.Text,
		Score:     score,
	}
}

func sortSearchResults(results []SearchResult) {
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].MessageID.String() < results[j].MessageID.String()
		}
		return results[i].Score > results[j].Score
	})
}

func limitSearchResults(results []SearchResult, limit int) []SearchResult {
	if limit <= 0 || len(results) <= limit {
		return results
	}
	return results[:limit]
}

func decodeVector(blob []byte) []float32 {
	if len(blob) == 0 || len(blob)%4 != 0 {
		return nil
	}
	out := make([]float32, len(blob)/4)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4 : i*4+4]))
	}
	return out
}

func cosineScore(a []float32, b []float32) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		x := float64(a[i])
		y := float64(b[i])
		dot += x * y
		normA += x * x
		normB += y * y
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

func excerpt(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if maxRunes <= 0 || len([]rune(text)) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

func keywordExcerpt(text string, query string, maxRunes int) string {
	text = strings.TrimSpace(text)
	query = strings.TrimSpace(query)
	if text == "" || query == "" {
		return excerpt(text, maxRunes)
	}
	lowerText := strings.ToLower(text)
	lowerQuery := strings.ToLower(query)
	idx := strings.Index(lowerText, lowerQuery)
	if idx < 0 {
		return excerpt(text, maxRunes)
	}
	runes := []rune(text)
	prefixRunes := len([]rune(text[:idx]))
	half := maxRunes / 2
	start := prefixRunes - half
	if start < 0 {
		start = 0
	}
	end := start + maxRunes
	if end > len(runes) {
		end = len(runes)
	}
	out := strings.TrimSpace(string(runes[start:end]))
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}
