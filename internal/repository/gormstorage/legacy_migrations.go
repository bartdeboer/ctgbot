package gormstorage

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// migrateComponentProfileColumn preserves existing registrations created before
// the component profile rename, when the components table used home_path.
func (s *GORMStorage) migrateComponentProfileColumn(ctx context.Context) error {
	migrator := s.db.WithContext(ctx).Migrator()
	model := &coremodel.Component{}
	if !migrator.HasTable(model) {
		return nil
	}
	if migrator.HasColumn(model, "home_path") && !migrator.HasColumn(model, "profile_path") {
		return migrator.RenameColumn(model, "home_path", "profile_path")
	}
	return nil
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
