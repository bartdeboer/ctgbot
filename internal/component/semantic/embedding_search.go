package semantic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

const defaultQueryInstruction = "Given a chat search query, retrieve relevant chat messages that answer or discuss the query."

type IndexRequest struct {
	Strategy    string
	ThreadID    modeluuid.UUID
	MaxMessages int
	BatchSize   int
}

type IndexResult struct {
	Messages int
	Embedded int
	Skipped  int
}

type StrategySearchRequest struct {
	Strategy string
	Query    string
	ThreadID modeluuid.UUID
	Limit    int
}

func (c *Component) IndexThread(ctx context.Context, req IndexRequest) (IndexResult, error) {
	if c == nil || c.store == nil {
		return IndexResult{}, fmt.Errorf("missing semantic store")
	}
	if c.messages == nil {
		return IndexResult{}, fmt.Errorf("missing semantic message source")
	}
	if req.ThreadID.IsNull() {
		return IndexResult{}, fmt.Errorf("missing thread id")
	}
	strategy, err := c.embeddingStrategy(ctx, req.Strategy)
	if err != nil {
		return IndexResult{}, err
	}
	embedder, _, err := c.resolveEmbedder(ctx, strategy.EmbedderRef)
	if err != nil {
		return IndexResult{}, err
	}
	messages, err := c.messages.ThreadMessages(ctx, req.ThreadID)
	if err != nil {
		return IndexResult{}, err
	}
	items := searchableMessages(messages)
	if req.MaxMessages > 0 && len(items) > req.MaxMessages {
		items = items[len(items)-req.MaxMessages:]
	}
	result := IndexResult{Messages: len(items)}
	batchSize := req.BatchSize
	if batchSize <= 0 {
		batchSize = strategy.BatchSize
	}
	if batchSize <= 0 {
		batchSize = c.config.EmbeddingBatchSize
	}
	if batchSize <= 0 {
		batchSize = DefaultEmbeddingBatchSize
	}

	var batch []coremodel.ThreadMessage
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		inputs := make([]component.EmbeddingInput, 0, len(batch))
		for _, message := range batch {
			inputs = append(inputs, component.EmbeddingInput{ID: message.ID.String(), Text: message.Text, Kind: component.EmbeddingKindDocument})
		}
		response, err := embedder.Embed(ctx, component.EmbedRequest{Model: strategy.Model, Inputs: inputs})
		if err != nil {
			return err
		}
		byID := map[string]coremodel.ThreadMessage{}
		for _, message := range batch {
			byID[message.ID.String()] = message
		}
		for _, embedding := range response.Embeddings {
			message, ok := byID[strings.TrimSpace(embedding.ID)]
			if !ok {
				continue
			}
			hash := textHash(message.Text)
			existing, err := c.store.embedding(ctx, strategy.Name, strategySourceMessages, message.ID.String())
			if err != nil {
				return err
			}
			record := indexedEmbedding{
				StrategyName:   strategy.Name,
				SourceType:     strategySourceMessages,
				SourceID:       message.ID.String(),
				SourceTextHash: hash,
				ChatID:         message.ChatID.String(),
				ThreadID:       message.ThreadID.String(),
				Model:          embedding.Model,
				Dim:            embedding.Dim,
				Normalized:     embedding.Normalized,
				Vector:         encodeVector(embedding.Vector),
			}
			if existing != nil {
				record.ID = existing.ID
			}
			if err := c.store.saveEmbedding(ctx, record); err != nil {
				return err
			}
			result.Embedded++
		}
		batch = batch[:0]
		return nil
	}

	for _, message := range items {
		hash := textHash(message.Text)
		if err := c.store.saveMessage(ctx, indexedMessage{
			ID:        message.ID.String(),
			ChatID:    message.ChatID.String(),
			ThreadID:  message.ThreadID.String(),
			Text:      message.Text,
			TextHash:  hash,
			CreatedAt: message.CreatedAt,
			UpdatedAt: message.UpdatedAt,
		}); err != nil {
			return IndexResult{}, err
		}
		existing, err := c.store.embedding(ctx, strategy.Name, strategySourceMessages, message.ID.String())
		if err != nil {
			return IndexResult{}, err
		}
		if existing != nil && existing.SourceTextHash == hash && len(existing.Vector) > 0 {
			result.Skipped++
			continue
		}
		batch = append(batch, message)
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return IndexResult{}, err
			}
		}
	}
	if err := flush(); err != nil {
		return IndexResult{}, err
	}
	return result, nil
}

func (c *Component) SearchStrategy(ctx context.Context, req StrategySearchRequest) (component.SearchResponse, error) {
	if c == nil || c.store == nil {
		return component.SearchResponse{}, fmt.Errorf("missing semantic store")
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return component.SearchResponse{}, fmt.Errorf("missing search query")
	}
	if req.ThreadID.IsNull() {
		return component.SearchResponse{}, fmt.Errorf("missing thread id")
	}
	strategy, err := c.embeddingStrategy(ctx, req.Strategy)
	if err != nil {
		return component.SearchResponse{}, err
	}
	embedder, _, err := c.resolveEmbedder(ctx, strategy.EmbedderRef)
	if err != nil {
		return component.SearchResponse{}, err
	}
	queryText := embeddingQueryText(strategy, query)
	response, err := embedder.Embed(ctx, component.EmbedRequest{Model: strategy.Model, Inputs: []component.EmbeddingInput{{ID: "query", Text: queryText, Kind: component.EmbeddingKindQuery}}})
	if err != nil {
		return component.SearchResponse{}, err
	}
	if len(response.Embeddings) == 0 {
		return component.SearchResponse{}, fmt.Errorf("embed query returned no vector")
	}
	queryVector := response.Embeddings[0].Vector
	if len(queryVector) == 0 {
		return component.SearchResponse{}, fmt.Errorf("embed query returned empty vector")
	}
	embeddings, err := c.store.embeddingsForThread(ctx, strategy.Name, req.ThreadID.String())
	if err != nil {
		return component.SearchResponse{}, err
	}
	ids := make([]string, 0, len(embeddings))
	for _, embedding := range embeddings {
		if embedding.SourceType == strategySourceMessages {
			ids = append(ids, embedding.SourceID)
		}
	}
	messages, err := c.store.messagesByIDs(ctx, ids)
	if err != nil {
		return component.SearchResponse{}, err
	}
	results := make([]component.SearchResult, 0, len(embeddings))
	for _, embedding := range embeddings {
		vector := decodeVector(embedding.Vector)
		if len(vector) != len(queryVector) {
			continue
		}
		message, ok := messages[embedding.SourceID]
		if !ok {
			continue
		}
		results = append(results, component.SearchResult{
			MessageID: parseUUID(message.ID),
			ChatID:    parseUUID(message.ChatID),
			ThreadID:  parseUUID(message.ThreadID),
			Excerpt:   excerpt(message.Text, 240),
			Text:      message.Text,
			Score:     float64(dotProduct(queryVector, vector)),
		})
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].MessageID.String() < results[j].MessageID.String()
		}
		return results[i].Score > results[j].Score
	})
	limit := req.Limit
	if limit <= 0 {
		limit = c.config.Limit
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return component.SearchResponse{Results: results}, nil
}

func (c *Component) embeddingStrategy(ctx context.Context, name string) (*strategy, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("missing semantic store")
	}
	strategy, err := c.store.strategy(ctx, name)
	if err != nil {
		return nil, err
	}
	if strategy == nil {
		return nil, fmt.Errorf("semantic strategy not found: %s", normalizeStrategyName(name))
	}
	if strategy.Type != strategyTypeEmbedding {
		return nil, fmt.Errorf("semantic strategy %s is %s, not embedding", strategy.Name, strategy.Type)
	}
	if !strategy.Enabled {
		return nil, fmt.Errorf("semantic strategy disabled: %s", strategy.Name)
	}
	if strings.TrimSpace(strategy.EmbedderRef) == "" {
		return nil, fmt.Errorf("semantic strategy %s has no embedder", strategy.Name)
	}
	if strings.TrimSpace(strategy.Model) == "" {
		return nil, fmt.Errorf("semantic strategy %s has no model", strategy.Name)
	}
	return strategy, nil
}

func embeddingQueryText(strategy *strategy, query string) string {
	prompt := ""
	if strategy != nil {
		prompt = strings.TrimSpace(strategy.Prompt)
	}
	query = strings.TrimSpace(query)
	if prompt == "" {
		return query
	}
	return "Instruct: " + prompt + "\nQuery: " + query
}

func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum[:])
}

func encodeVector(vector []float32) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, vector)
	return buf.Bytes()
}

func decodeVector(blob []byte) []float32 {
	if len(blob) == 0 || len(blob)%4 != 0 {
		return nil
	}
	out := make([]float32, len(blob)/4)
	_ = binary.Read(bytes.NewReader(blob), binary.LittleEndian, out)
	return out
}

func dotProduct(a []float32, b []float32) float32 {
	var score float32
	for i := range a {
		score += a[i] * b[i]
	}
	if math.IsNaN(float64(score)) || math.IsInf(float64(score), 0) {
		return 0
	}
	return score
}

func parseUUID(value string) modeluuid.UUID {
	id, err := modeluuid.Parse(value)
	if err != nil {
		return modeluuid.UUID{}
	}
	return id
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
