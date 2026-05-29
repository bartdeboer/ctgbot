package indexing

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

const StoreFilename = "indexing.db"

const (
	StrategyTypeSummary   = "summary"
	StrategyTypeEmbedding = "embedding"
)

const (
	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
)

type store struct{ db *gorm.DB }

type indexStrategy struct {
	ID          string `gorm:"primaryKey"`
	Name        string `gorm:"uniqueIndex"`
	Type        string `gorm:"index"`
	ProviderRef string
	Model       string
	Prompt      string
	TargetChars int
	BatchSize   int
	Enabled     bool `gorm:"index"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (indexStrategy) TableName() string { return "index_strategies" }

type indexRun struct {
	ID           string `gorm:"primaryKey"`
	StrategyID   string `gorm:"index"`
	StrategyName string `gorm:"index"`
	Type         string `gorm:"index"`
	Status       string `gorm:"index"`
	StartedAt    time.Time
	FinishedAt   *time.Time
	ItemsSeen    int
	ItemsCreated int
	ItemsUpdated int
	ItemsSkipped int
	Error        string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (indexRun) TableName() string { return "index_runs" }

type messageSummary struct {
	ID         string `gorm:"primaryKey"`
	MessageID  string `gorm:"uniqueIndex:idx_summary_message_strategy;index"`
	StrategyID string `gorm:"uniqueIndex:idx_summary_message_strategy;index"`
	ChatID     string `gorm:"index"`
	ThreadID   string `gorm:"index"`
	SourceHash string `gorm:"index"`
	Summary    string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (messageSummary) TableName() string { return "message_summaries" }

type messageEmbedding struct {
	ID         string `gorm:"primaryKey"`
	MessageID  string `gorm:"uniqueIndex:idx_embedding_message_strategy;index"`
	StrategyID string `gorm:"uniqueIndex:idx_embedding_message_strategy;index"`
	ChatID     string `gorm:"index"`
	ThreadID   string `gorm:"index"`
	SourceHash string `gorm:"index"`
	Model      string
	Dim        int
	Normalized bool
	Embedding  []byte
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (messageEmbedding) TableName() string { return "message_embeddings" }

type stats struct {
	Strategies int64
	Runs       int64
	Summaries  int64
	Embeddings int64
}

func openStore(homePath string) (*store, error) {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil, fmt.Errorf("missing indexing profile home")
	}
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(homePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, fmt.Errorf("open indexing db: %w", err)
	}
	if err := db.AutoMigrate(&indexStrategy{}, &indexRun{}, &messageSummary{}, &messageEmbedding{}); err != nil {
		return nil, fmt.Errorf("migrate indexing db: %w", err)
	}
	return &store{db: db}, nil
}

func newID() string { return modeluuid.New().String() }

func normalizeName(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func (s *store) saveStrategy(ctx context.Context, strategy *indexStrategy) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("missing indexing store")
	}
	if strategy == nil {
		return fmt.Errorf("missing indexing strategy")
	}
	strategy.Name = normalizeName(strategy.Name)
	strategy.Type = strings.TrimSpace(strategy.Type)
	strategy.ProviderRef = strings.TrimSpace(strategy.ProviderRef)
	strategy.Model = strings.TrimSpace(strategy.Model)
	strategy.Prompt = strings.TrimSpace(strategy.Prompt)
	if strategy.Name == "" {
		return fmt.Errorf("missing strategy name")
	}
	if strategy.Type != StrategyTypeSummary && strategy.Type != StrategyTypeEmbedding {
		return fmt.Errorf("unsupported strategy type: %s", strategy.Type)
	}
	if strategy.ID == "" {
		existing, err := s.strategyByName(ctx, strategy.Name)
		if err != nil {
			return err
		}
		if existing != nil {
			strategy.ID = existing.ID
		}
	}
	if strategy.ID == "" {
		strategy.ID = newID()
	}
	strategy.Enabled = true
	return s.db.WithContext(ctx).Save(strategy).Error
}

func (s *store) strategyByName(ctx context.Context, name string) (*indexStrategy, error) {
	var strategy indexStrategy
	err := s.db.WithContext(ctx).First(&strategy, "name = ?", normalizeName(name)).Error
	if err == nil {
		return &strategy, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) listStrategies(ctx context.Context) ([]indexStrategy, error) {
	var out []indexStrategy
	return out, s.db.WithContext(ctx).Order("name asc").Find(&out).Error
}

func (s *store) summary(ctx context.Context, strategyID string, messageID string) (*messageSummary, error) {
	var summary messageSummary
	err := s.db.WithContext(ctx).Where("strategy_id = ? AND message_id = ?", strategyID, messageID).First(&summary).Error
	if err == nil {
		return &summary, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) saveSummary(ctx context.Context, summary messageSummary) error {
	if summary.ID == "" {
		summary.ID = newID()
	}
	return s.db.WithContext(ctx).Save(&summary).Error
}

func (s *store) embedding(ctx context.Context, strategyID string, messageID string) (*messageEmbedding, error) {
	var embedding messageEmbedding
	err := s.db.WithContext(ctx).Where("strategy_id = ? AND message_id = ?", strategyID, messageID).First(&embedding).Error
	if err == nil {
		return &embedding, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, err
}

func (s *store) saveEmbedding(ctx context.Context, embedding messageEmbedding) error {
	if embedding.ID == "" {
		embedding.ID = newID()
	}
	return s.db.WithContext(ctx).Save(&embedding).Error
}

func (s *store) createRun(ctx context.Context, strategy indexStrategy) (*indexRun, error) {
	run := indexRun{ID: newID(), StrategyID: strategy.ID, StrategyName: strategy.Name, Type: strategy.Type, Status: RunStatusRunning, StartedAt: time.Now().UTC()}
	return &run, s.db.WithContext(ctx).Save(&run).Error
}

func (s *store) finishRun(ctx context.Context, run *indexRun, err error) error {
	if run == nil {
		return nil
	}
	finished := time.Now().UTC()
	run.FinishedAt = &finished
	if err != nil {
		run.Status = RunStatusFailed
		run.Error = err.Error()
	} else {
		run.Status = RunStatusSuccess
		run.Error = ""
	}
	return s.db.WithContext(ctx).Save(run).Error
}

func (s *store) stats(ctx context.Context) (stats, error) {
	var out stats
	if err := s.db.WithContext(ctx).Model(&indexStrategy{}).Count(&out.Strategies).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&indexRun{}).Count(&out.Runs).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&messageSummary{}).Count(&out.Summaries).Error; err != nil {
		return out, err
	}
	if err := s.db.WithContext(ctx).Model(&messageEmbedding{}).Count(&out.Embeddings).Error; err != nil {
		return out, err
	}
	return out, nil
}
