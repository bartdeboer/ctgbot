package semantic

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const StoreFilename = "semantic.db"

const (
	strategyTypeEmbedding  = "embedding"
	strategyTypeDerivation = "derivation"

	strategySourceMessages    = "messages"
	strategySourceDerivations = "derivations"
)

type store struct{ db *gorm.DB }

type strategy struct {
	Name           string `gorm:"primaryKey"`
	Type           string `gorm:"index"`
	SourceKind     string `gorm:"index"`
	SourceStrategy string `gorm:"index"`
	EmbedderRef    string
	CompletionRef  string
	Model          string
	Prompt         string
	BatchSize      int
	Enabled        bool `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (strategy) TableName() string { return "strategies" }

type indexedMessage struct {
	ID        string `gorm:"primaryKey"`
	ChatID    string `gorm:"index"`
	ThreadID  string `gorm:"index"`
	Text      string
	TextHash  string `gorm:"index"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (indexedMessage) TableName() string { return "messages" }

type derivation struct {
	ID             string `gorm:"primaryKey"`
	MessageID      string `gorm:"index"`
	StrategyName   string `gorm:"index"`
	Kind           string `gorm:"index"`
	Text           string
	TextHash       string `gorm:"index"`
	SourceTextHash string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (derivation) TableName() string { return "derivations" }

type indexedEmbedding struct {
	ID             string `gorm:"primaryKey"`
	StrategyName   string `gorm:"index:idx_embedding_source,unique;index"`
	SourceType     string `gorm:"index:idx_embedding_source,unique"`
	SourceID       string `gorm:"index:idx_embedding_source,unique;index"`
	SourceTextHash string `gorm:"index"`
	ChatID         string `gorm:"index"`
	ThreadID       string `gorm:"index"`
	Model          string
	Dim            int
	Normalized     bool
	Vector         []byte
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (indexedEmbedding) TableName() string { return "embeddings" }

type semanticStats struct {
	Strategies  int64
	Messages    int64
	Derivations int64
	Embeddings  int64
}

func openStore(homePath string) (*store, error) {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil, fmt.Errorf("missing semantic profile home")
	}
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(homePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open semantic db: %w", err)
	}
	if err := db.AutoMigrate(&strategy{}, &indexedMessage{}, &derivation{}, &indexedEmbedding{}); err != nil {
		return nil, fmt.Errorf("migrate semantic db: %w", err)
	}
	return &store{db: db}, nil
}

func newStoreID() string { return modeluuid.New().String() }

func (s *store) saveStrategy(ctx context.Context, strategy *strategy) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing semantic store")
	}
	if strategy == nil {
		return fmt.Errorf("missing semantic strategy")
	}
	strategy.Name = normalizeStrategyName(strategy.Name)
	strategy.Type = strings.TrimSpace(strategy.Type)
	strategy.SourceKind = strings.TrimSpace(strategy.SourceKind)
	strategy.SourceStrategy = normalizeStrategyName(strategy.SourceStrategy)
	strategy.EmbedderRef = strings.TrimSpace(strategy.EmbedderRef)
	strategy.CompletionRef = strings.TrimSpace(strategy.CompletionRef)
	strategy.Model = strings.TrimSpace(strategy.Model)
	strategy.Prompt = strings.TrimSpace(strategy.Prompt)
	if strategy.Name == "" {
		return fmt.Errorf("missing strategy name")
	}
	if strategy.Type == "" {
		return fmt.Errorf("missing strategy type")
	}
	if strategy.SourceKind == "" {
		strategy.SourceKind = strategySourceMessages
	}
	strategy.Enabled = true
	return s.db.WithContext(ctx).Save(strategy).Error
}

func (s *store) strategy(ctx context.Context, name string) (*strategy, error) {
	var strategy strategy
	err := s.db.WithContext(ctx).First(&strategy, "name = ?", normalizeStrategyName(name)).Error
	if err == nil {
		return &strategy, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) listStrategies(ctx context.Context) ([]strategy, error) {
	var out []strategy
	return out, s.db.WithContext(ctx).Order("name asc").Find(&out).Error
}

func (s *store) saveMessage(ctx context.Context, message indexedMessage) error {
	if strings.TrimSpace(message.ID) == "" {
		return fmt.Errorf("missing semantic message id")
	}
	return s.db.WithContext(ctx).Save(&message).Error
}

func (s *store) embedding(ctx context.Context, strategyName string, sourceType string, sourceID string) (*indexedEmbedding, error) {
	var embedding indexedEmbedding
	err := s.db.WithContext(ctx).
		Where("strategy_name = ? AND source_type = ? AND source_id = ?", normalizeStrategyName(strategyName), strings.TrimSpace(sourceType), strings.TrimSpace(sourceID)).
		First(&embedding).Error
	if err == nil {
		return &embedding, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) saveEmbedding(ctx context.Context, embedding indexedEmbedding) error {
	if strings.TrimSpace(embedding.ID) == "" {
		embedding.ID = newStoreID()
	}
	return s.db.WithContext(ctx).Save(&embedding).Error
}

func (s *store) embeddingsForThread(ctx context.Context, strategyName string, threadID string) ([]indexedEmbedding, error) {
	var out []indexedEmbedding
	return out, s.db.WithContext(ctx).
		Where("strategy_name = ? AND thread_id = ?", normalizeStrategyName(strategyName), strings.TrimSpace(threadID)).
		Order("created_at asc, id asc").
		Find(&out).Error
}

func (s *store) messagesByIDs(ctx context.Context, ids []string) (map[string]indexedMessage, error) {
	out := map[string]indexedMessage{}
	if len(ids) == 0 {
		return out, nil
	}
	var messages []indexedMessage
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&messages).Error; err != nil {
		return nil, err
	}
	for _, message := range messages {
		out[message.ID] = message
	}
	return out, nil
}

func (s *store) stats(ctx context.Context) (semanticStats, error) {
	var stats semanticStats
	if err := s.db.WithContext(ctx).Model(&strategy{}).Count(&stats.Strategies).Error; err != nil {
		return stats, err
	}
	if err := s.db.WithContext(ctx).Model(&indexedMessage{}).Count(&stats.Messages).Error; err != nil {
		return stats, err
	}
	if err := s.db.WithContext(ctx).Model(&derivation{}).Count(&stats.Derivations).Error; err != nil {
		return stats, err
	}
	if err := s.db.WithContext(ctx).Model(&indexedEmbedding{}).Count(&stats.Embeddings).Error; err != nil {
		return stats, err
	}
	return stats, nil
}

func normalizeStrategyName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
