package appstate

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/go-clistate"
)

const CodexLoginCallbackPort = 1455

func NewConfig(root string, store *clistate.Store, globalStore ...*clistate.Store) (*Config, error) {
	if strings.TrimSpace(root) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(cwd, ".ctgbot")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return New(absRoot, store, globalStore...), nil
}
