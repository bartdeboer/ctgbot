package broker

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
)

type turnData struct{ app App }

func (d turnData) ForEachMessage(ctx context.Context, scope component.MessageScope, visit component.MessageVisitor) error {
	if d.app == nil {
		return nil
	}
	return d.app.ForEachMessage(ctx, scope, visit)
}
