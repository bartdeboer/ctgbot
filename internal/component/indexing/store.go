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
	ID             string `gorm:"primaryKey"`
	Name           string `gorm:"uniqueIndex"`
	Type           string `gorm:"index"`
	ProviderRef    string
	Model          string
	Prompt         string
	TargetChars    int
	CopyUnderChars int
	BatchSize      int
	Enabled        bool `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
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

type clearResult struct {
	Strategy   string
	Runs       int64
	Summaries  int64
	Embeddings int64
}

func openStore(profilePath string) (*store, error) {
	profilePath = strings.TrimSpace(profilePath)
	if profilePath == "" {
		return nil, fmt.Errorf("missing indexing profile")
	}
	if err := os.MkdirAll(profilePath, 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(filepath.Join(profilePath, StoreFilename)), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
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

func (s *store) searchEmbeddings(ctx context.Context, strategyID string, scope scope) ([]messageEmbedding, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("missing indexing store")
	}
	query := s.db.WithContext(ctx).Where("strategy_id = ?", strategyID)
	if !scope.ChatID.IsNull() {
		query = query.Where("chat_id = ?", scope.ChatID.String())
	}
	if !scope.ThreadID.IsNull() {
		query = query.Where("thread_id = ?", scope.ThreadID.String())
	}
	var out []messageEmbedding
	return out, query.Find(&out).Error
}

func (s *store) summariesByMessageID(ctx context.Context, strategyName string, messageIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if s == nil || s.db == nil {
		return out, fmt.Errorf("missing indexing store")
	}
	strategyName = normalizeName(strategyName)
	if strategyName == "" || len(messageIDs) == 0 {
		return out, nil
	}
	strategy, err := s.strategyByName(ctx, strategyName)
	if err != nil {
		return out, err
	}
	if strategy == nil || strategy.Type != StrategyTypeSummary {
		return out, nil
	}
	ids := compactStrings(messageIDs)
	if len(ids) == 0 {
		return out, nil
	}
	var summaries []messageSummary
	if err := s.db.WithContext(ctx).Where("strategy_id = ? AND message_id IN ?", strategy.ID, ids).Find(&summaries).Error; err != nil {
		return out, err
	}
	for _, summary := range summaries {
		out[summary.MessageID] = strings.TrimSpace(summary.Summary)
	}
	return out, nil
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

func (s *store) clearStrategy(ctx context.Context, name string) (clearResult, error) {
	if s == nil || s.db == nil {
		return clearResult{}, fmt.Errorf("missing indexing store")
	}
	strategy, err := s.strategyByName(ctx, name)
	if err != nil {
		return clearResult{}, err
	}
	if strategy == nil {
		return clearResult{}, fmt.Errorf("indexing strategy not found: %s", normalizeName(name))
	}
	var out clearResult
	out.Strategy = strategy.Name
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Where("strategy_id = ?", strategy.ID).Delete(&messageEmbedding{})
		if result.Error != nil {
			return result.Error
		}
		out.Embeddings = result.RowsAffected

		result = tx.Where("strategy_id = ?", strategy.ID).Delete(&messageSummary{})
		if result.Error != nil {
			return result.Error
		}
		out.Summaries = result.RowsAffected

		result = tx.Where("strategy_id = ?", strategy.ID).Delete(&indexRun{})
		if result.Error != nil {
			return result.Error
		}
		out.Runs = result.RowsAffected
		return nil
	})
	return out, err
}
