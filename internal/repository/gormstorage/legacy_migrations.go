package gormstorage

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// ensureLegacyComponentProfileShadow keeps the profile rename rollback-friendly.
//
// During the transition, profile_path is the new canonical column but home_path
// remains as a legacy shadow column. That lets a smoke test on this branch be
// rolled back to current main without losing the latest component profile path.
func (s *GORMStorage) ensureLegacyComponentProfileShadow(ctx context.Context) error {
	migrator := s.db.WithContext(ctx).Migrator()
	model := &coremodel.Component{}
	if !migrator.HasTable(model) {
		return nil
	}
	if !migrator.HasColumn(model, "profile_path") {
		if err := migrator.AddColumn(model, "ProfilePath"); err != nil {
			return err
		}
	}
	if !migrator.HasColumn(model, "home_path") {
		if err := s.db.WithContext(ctx).Exec(`ALTER TABLE components ADD COLUMN home_path TEXT`).Error; err != nil {
			return err
		}
	}
	if err := s.db.WithContext(ctx).Exec(`
		UPDATE components
		SET profile_path = home_path
		WHERE COALESCE(TRIM(profile_path), '') = ''
		  AND COALESCE(TRIM(home_path), '') <> ''
	`).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Exec(`
		UPDATE components
		SET home_path = profile_path
		WHERE COALESCE(TRIM(home_path), '') = ''
		  AND COALESCE(TRIM(profile_path), '') <> ''
	`).Error; err != nil {
		return err
	}
	return nil
}

func (r *gormComponents) applyLegacyComponentProfileFallback(ctx context.Context, component *coremodel.Component) error {
	if component == nil || component.ID.IsNull() || strings.TrimSpace(component.ProfilePath) != "" {
		return nil
	}
	if !r.hasLegacyComponentHomePath(ctx) {
		return nil
	}
	var row struct {
		HomePath string `gorm:"column:home_path"`
	}
	if err := r.db.WithContext(ctx).
		Table("components").
		Select("home_path").
		Where("id = ?", component.ID).
		Scan(&row).Error; err != nil {
		return err
	}
	component.ProfilePath = clean(row.HomePath)
	return nil
}

func (r *gormComponents) writeLegacyComponentProfileShadow(ctx context.Context, componentID modeluuid.UUID, profilePath string) error {
	if componentID.IsNull() || !r.hasLegacyComponentHomePath(ctx) {
		return nil
	}
	return r.db.WithContext(ctx).
		Table("components").
		Where("id = ?", componentID).
		Update("home_path", clean(profilePath)).
		Error
}

func (r *gormComponents) hasLegacyComponentHomePath(ctx context.Context) bool {
	if r == nil || r.db == nil {
		return false
	}
	return r.db.WithContext(ctx).Migrator().HasColumn(&coremodel.Component{}, "home_path")
}

// migrateProviderChannelColumns preserves source bindings and drops created
// before provider/external addresses were renamed from chat IDs to channel IDs.
func (s *GORMStorage) migrateProviderChannelColumns(ctx context.Context) error {
	migrator := s.db.WithContext(ctx).Migrator()
	for _, migration := range []struct {
		model any
		old   string
		new   string
	}{
		{model: &coremodel.ChatComponent{}, old: "external_chat_id", new: "external_channel_id"},
		{model: &coremodel.InboundDrop{}, old: "external_chat_id", new: "external_channel_id"},
	} {
		if !migrator.HasTable(migration.model) {
			continue
		}
		if migrator.HasColumn(migration.model, migration.old) && !migrator.HasColumn(migration.model, migration.new) {
			if err := migrator.RenameColumn(migration.model, migration.old, migration.new); err != nil {
				return err
			}
		}
	}
	return nil
}
