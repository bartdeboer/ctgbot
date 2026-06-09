package theater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ensureWorkspace(path string, name string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(path, "SKILLS"), 0o755); err != nil {
		return fmt.Errorf("create theater workspace: %w", err)
	}
	if err := writeFileIfMissing(filepath.Join(path, "README.md"), defaultReadme(name)); err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(path, "AGENTS.md"), defaultAgents(name)); err != nil {
		return err
	}
	return nil
}

func writeFileIfMissing(path string, text string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(text)+"\n"), 0o644)
}

func defaultReadme(name string) string {
	name = normalizeName(name)
	if name == "" {
		name = "theater"
	}
	return fmt.Sprintf(`# %s

Shared context for the %s theater.

Use this folder for durable notes, references, and artifacts that should survive message history and context compaction.`, name, name)
}

func defaultAgents(name string) string {
	name = normalizeName(name)
	if name == "" {
		name = "theater"
	}
	return fmt.Sprintf(`# %s agents

Document participating agents, working norms, responsibilities, and useful local conventions here.`, name)
}
