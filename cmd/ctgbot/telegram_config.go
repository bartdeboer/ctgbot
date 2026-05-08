package main

import (
	"os"
	"strings"

	"github.com/bartdeboer/go-clistate"
)

func resolveTelegramToken(flagVal string, store *clistate.Store) string {
	if value := strings.TrimSpace(flagVal); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); value != "" {
		return value
	}
	if store == nil {
		return ""
	}
	return strings.TrimSpace(store.GetString("telegram.token", ""))
}
