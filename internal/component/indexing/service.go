package indexing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

const defaultSummaryPrompt = "Summarize this chat message for compact future context. Preserve concrete facts, decisions, tasks, filenames, commands, and user preferences. Do not add information."

const summaryRunIdleTimeout = 5 * time.Minute

type RunRequest struct {
	Strategy    string
	Scope       scope
	MaxMessages int
	BatchSize   int
}

type RunResult struct {
	Strategy    string
	Type        string
	Messages    int
	Created     int
	Updated     int
	Skipped     int
	RunID       string
	ProviderRef string
}

func (c *Component) RunStrategy(ctx context.Context, req RunRequest) (RunResult, error) {
	if c == nil || c.store == nil {
		return RunResult{}, fmt.Errorf("missing indexing store")
	}
	strategy, err := c.store.strategyByName(ctx, req.Strategy)
	if err != nil {
		return RunResult{}, err
	}
	if strategy == nil {
		return RunResult{}, fmt.Errorf("index strategy not found: %s", normalizeName(req.Strategy))
	}
	if !strategy.Enabled {
		return RunResult{}, fmt.Errorf("index strategy disabled: %s", strategy.Name)
	}
	run, err := c.store.createRun(ctx, *strategy)
	if err != nil {
		return RunResult{}, err
	}
	result, runErr := c.runStrategy(ctx, *strategy, run, req)
	if finishErr := c.store.finishRun(ctx, run, runErr); finishErr != nil && runErr == nil {
		runErr = finishErr
	}
	result.RunID = run.ID
	if runErr != nil {
		return result, runErr
	}
	return result, nil
}

func (c *Component) runStrategy(ctx context.Context, strategy indexStrategy, run *indexRun, req RunRequest) (RunResult, error) {
	messages, err := c.messagesForScope(ctx, req.Scope, req.MaxMessages)
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{Strategy: strategy.Name, Type: strategy.Type, Messages: len(messages)}
	if run != nil {
		run.ItemsSeen = len(messages)
	}
	switch strategy.Type {
	case StrategyTypeSummary:
		engine, engineRef, err := c.resolveCompletionEngine(ctx, strategy.ProviderRef)
		if err != nil {
			return result, err
		}
		result.ProviderRef = engineRef
		return c.runSummaryStrategy(ctx, engine, strategy, run, req, messages, result)
	case StrategyTypeEmbedding:
		embedder, engineRef, err := c.resolveEmbeddingEngine(ctx, strategy.ProviderRef)
		if err != nil {
			return result, err
		}
		result.ProviderRef = engineRef
		return c.runEmbeddingStrategy(ctx, embedder, strategy, run, req, messages, result)
	default:
		return result, fmt.Errorf("unsupported strategy type: %s", strategy.Type)
	}
}

func (c *Component) messagesForScope(ctx context.Context, scope scope, maxMessages int) ([]coremodel.ThreadMessage, error) {
	if c == nil || c.messages == nil {
		return nil, fmt.Errorf("missing indexing message source")
	}
	if maxMessages > 0 {
		scope.Limit = maxMessages
		scope.Order = component.MessageOrderNewestFirst
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

func searchableMessage(message coremodel.ThreadMessage) bool {
	if strings.TrimSpace(message.Text) == "" || message.ID.IsNull() {
		return false
	}
	return true
}

func indexableMessageKinds() []coremodel.MessageKind {
	return []coremodel.MessageKind{
		coremodel.MessageKindMessage,
		// Legacy rows before the Role/Kind split stored the conversational role
		// in Kind. Keep them indexable until old databases are backfilled.
		coremodel.MessageKind("user"),
		coremodel.MessageKind("agent"),
	}
}

func (c *Component) runSummaryStrategy(ctx context.Context, engine component.CompletionEngine, strategy indexStrategy, run *indexRun, req RunRequest, messages []coremodel.ThreadMessage, result RunResult) (RunResult, error) {
	var session component.InferenceSession
	defer func() {
		if session != nil {
			_ = session.Close()
		}
	}()
	for _, message := range messages {
		hash := textHash(message.Text)
		existing, err := c.store.summary(ctx, strategy.ID, message.ID.String())
		if err != nil {
			return result, err
		}
		if existing != nil && existing.SourceHash == hash && strings.TrimSpace(existing.Summary) != "" {
			result.Skipped++
			continue
		}
		if !shouldCopySummaryVerbatim(strategy, strings.TrimSpace(message.Text)) && session == nil {
			session, err = beginSummaryRunSession(ctx, engine, strategy.Model)
			if err != nil {
				return result, err
			}
		}
		summary, err := c.summarizeMessage(ctx, engine, strategy, message)
		if err != nil {
			return result, err
		}
		record := messageSummary{
			MessageID:  message.ID.String(),
			StrategyID: strategy.ID,
			ChatID:     message.ChatID.String(),
			ThreadID:   message.ThreadID.String(),
			SourceHash: hash,
			Summary:    strings.TrimSpace(summary),
		}
		if existing != nil {
			record.ID = existing.ID
			result.Updated++
		} else {
			result.Created++
		}
		if err := c.store.saveSummary(ctx, record); err != nil {
			return result, err
		}
	}
	copyRunCounts(run, result)
	return result, nil
}

func beginSummaryRunSession(ctx context.Context, engine component.CompletionEngine, model string) (component.InferenceSession, error) {
	sessionEngine, ok := engine.(component.InferenceSessionEngine)
	if !ok {
		return nil, nil
	}
	return sessionEngine.BeginInferenceSession(ctx, component.InferenceSessionOptions{Model: strings.TrimSpace(model), IdleTimeout: summaryRunIdleTimeout})
}

func (c *Component) summarizeMessage(ctx context.Context, engine component.CompletionEngine, strategy indexStrategy, message coremodel.ThreadMessage) (string, error) {
	text := strings.TrimSpace(message.Text)
	if shouldCopySummaryVerbatim(strategy, text) {
		return text, nil
	}
	targetChars := strategy.TargetChars
	if targetChars <= 0 {
		targetChars = 500
	}
	prompt := strings.TrimSpace(strategy.Prompt)
	if prompt == "" {
		prompt = defaultSummaryPrompt
	}
	content := fmt.Sprintf("%s\n\nTarget: at most %d characters.\nRole: %s\nMessage:\n%s", prompt, targetChars, message.ResolvedRole(), text)
	result, err := engine.Complete(ctx, component.CompletionRequest{
		Model: strings.TrimSpace(strategy.Model),
		Prompt: component.CompletionPrompt{Messages: []component.CompletionMessage{{
			Role:    component.CompletionRoleUser,
			Content: content,
		}}},
		MaxOutputTokens: max(64, targetChars/3),
		Reasoning:       component.ReasoningDisabled,
	})
	if err != nil {
		return "", err
	}
	if result == nil || result.Final == nil {
		return "", fmt.Errorf("summary completion returned no final message")
	}
	return strings.TrimSpace(result.Final.Text), nil
}

func shouldCopySummaryVerbatim(strategy indexStrategy, text string) bool {
	return strategy.CopyUnderChars > 0 && runeCount(text) <= strategy.CopyUnderChars
}

func runeCount(text string) int { return len([]rune(text)) }

func (c *Component) runEmbeddingStrategy(ctx context.Context, embedder component.EmbeddingEngine, strategy indexStrategy, run *indexRun, req RunRequest, messages []coremodel.ThreadMessage, result RunResult) (RunResult, error) {
	batchSize := firstPositive(req.BatchSize, strategy.BatchSize, 128)
	var batch []coremodel.ThreadMessage
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		inputs := make([]component.EmbeddingInput, 0, len(batch))
		byID := map[string]coremodel.ThreadMessage{}
		for _, message := range batch {
			id := message.ID.String()
			byID[id] = message
			inputs = append(inputs, component.EmbeddingInput{ID: id, Text: message.Text, Kind: component.EmbeddingKindDocument})
		}
		response, err := embedder.Embed(ctx, component.EmbeddingRequest{Model: strategy.Model, Inputs: inputs})
		if err != nil {
			return err
		}
		for _, embedding := range response.Embeddings {
			message, ok := byID[strings.TrimSpace(embedding.ID)]
			if !ok {
				continue
			}
			hash := textHash(message.Text)
			existing, err := c.store.embedding(ctx, strategy.ID, message.ID.String())
			if err != nil {
				return err
			}
			record := messageEmbedding{
				MessageID:  message.ID.String(),
				StrategyID: strategy.ID,
				ChatID:     message.ChatID.String(),
				ThreadID:   message.ThreadID.String(),
				SourceHash: hash,
				Model:      embedding.Model,
				Dim:        embedding.Dim,
				Normalized: embedding.Normalized,
				Embedding:  encodeVector(embedding.Vector),
			}
			if existing != nil {
				record.ID = existing.ID
				result.Updated++
			} else {
				result.Created++
			}
			if err := c.store.saveEmbedding(ctx, record); err != nil {
				return err
			}
		}
		batch = batch[:0]
		return nil
	}
	for _, message := range messages {
		hash := textHash(message.Text)
		existing, err := c.store.embedding(ctx, strategy.ID, message.ID.String())
		if err != nil {
			return result, err
		}
		if existing != nil && existing.SourceHash == hash && len(existing.Embedding) > 0 {
			result.Skipped++
			continue
		}
		batch = append(batch, message)
		if len(batch) >= batchSize {
			if err := flush(); err != nil {
				return result, err
			}
		}
	}
	if err := flush(); err != nil {
		return result, err
	}
	copyRunCounts(run, result)
	return result, nil
}

func copyRunCounts(run *indexRun, result RunResult) {
	if run == nil {
		return
	}
	run.ItemsSeen = result.Messages
	run.ItemsCreated = result.Created
	run.ItemsUpdated = result.Updated
	run.ItemsSkipped = result.Skipped
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

func parseUUID(value string) modeluuid.UUID {
	id, err := modeluuid.Parse(value)
	if err != nil {
		return modeluuid.UUID{}
	}
	return id
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
