package applypatch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func apply(workspace string, patch Patch) (Result, error) {
	workspace, err := cleanWorkspace(workspace)
	if err != nil {
		return Result{}, err
	}
	delta := Delta{Exact: true}
	for _, hunk := range patch.Hunks {
		switch h := hunk.(type) {
		case AddFile:
			change, err := applyAdd(workspace, h)
			if err != nil {
				return Result{}, err
			}
			delta.Changes = append(delta.Changes, change)
		case DeleteFile:
			change, err := applyDelete(workspace, h)
			if err != nil {
				return Result{}, err
			}
			delta.Changes = append(delta.Changes, change)
		case UpdateFile:
			change, err := applyUpdate(workspace, h)
			if err != nil {
				return Result{}, err
			}
			delta.Changes = append(delta.Changes, change)
		default:
			return Result{}, fmt.Errorf("unsupported hunk %T", hunk)
		}
	}
	return Result{Summary: summarize(delta), Delta: delta}, nil
}

func applyAdd(workspace string, h AddFile) (Change, error) {
	path, err := resolvePath(workspace, h.Path)
	if err != nil {
		return Change{}, err
	}
	if _, err := os.Stat(path); err == nil {
		return Change{}, fmt.Errorf("add file already exists: %s", h.Path)
	} else if !os.IsNotExist(err) {
		return Change{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Change{}, err
	}
	if err := os.WriteFile(path, []byte(h.Content), 0o644); err != nil {
		return Change{}, err
	}
	return Change{Kind: "add", Path: h.Path, NewContent: h.Content}, nil
}

func applyDelete(workspace string, h DeleteFile) (Change, error) {
	path, err := resolvePath(workspace, h.Path)
	if err != nil {
		return Change{}, err
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return Change{}, fmt.Errorf("delete file %s: %w", h.Path, err)
	}
	if err := os.Remove(path); err != nil {
		return Change{}, err
	}
	return Change{Kind: "delete", Path: h.Path, OldContent: string(old)}, nil
}

func applyUpdate(workspace string, h UpdateFile) (Change, error) {
	path, err := resolvePath(workspace, h.Path)
	if err != nil {
		return Change{}, err
	}
	oldBytes, err := os.ReadFile(path)
	if err != nil {
		return Change{}, fmt.Errorf("update file %s: %w", h.Path, err)
	}
	oldContent := string(oldBytes)
	newContent := oldContent
	for _, chunk := range h.Chunks {
		var err error
		newContent, err = applyChunk(newContent, chunk, h.Path)
		if err != nil {
			return Change{}, err
		}
	}
	targetPath := path
	if strings.TrimSpace(h.MoveTo) != "" {
		targetPath, err = resolvePath(workspace, h.MoveTo)
		if err != nil {
			return Change{}, err
		}
		if _, err := os.Stat(targetPath); err == nil {
			return Change{}, fmt.Errorf("move target already exists: %s", h.MoveTo)
		} else if !os.IsNotExist(err) {
			return Change{}, err
		}
		if err := os.Remove(path); err != nil {
			return Change{}, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return Change{}, err
	}
	if err := os.WriteFile(targetPath, []byte(newContent), 0o644); err != nil {
		return Change{}, err
	}
	return Change{Kind: "update", Path: h.Path, MoveTo: h.MoveTo, OldContent: oldContent, NewContent: newContent}, nil
}

func applyChunk(content string, chunk UpdateChunk, path string) (string, error) {
	oldBlock := strings.Join(chunk.OldLines, "")
	newBlock := strings.Join(chunk.NewLines, "")
	if chunk.EndOfFile {
		idx := strings.LastIndex(content, oldBlock)
		if idx < 0 || idx+len(oldBlock) != len(content) {
			return "", fmt.Errorf("update hunk did not match end of file: %s", path)
		}
		return content[:idx] + newBlock, nil
	}
	first := strings.Index(content, oldBlock)
	if first < 0 {
		return "", fmt.Errorf("update hunk did not match: %s", path)
	}
	last := strings.LastIndex(content, oldBlock)
	if first != last {
		return "", fmt.Errorf("update hunk matched multiple locations: %s", path)
	}
	return content[:first] + newBlock + content[first+len(oldBlock):], nil
}

func cleanWorkspace(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		workspace = "."
	}
	abs, err := filepath.Abs(workspace)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func resolvePath(workspace string, rel string) (string, error) {
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." || rel == "" {
		return "", fmt.Errorf("empty patch path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute patch path rejected: %s", rel)
	}
	path := filepath.Join(workspace, rel)
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	prefix := workspace + string(os.PathSeparator)
	if abs != workspace && !strings.HasPrefix(abs, prefix) {
		return "", fmt.Errorf("patch path escapes workspace: %s", rel)
	}
	return abs, nil
}

func summarize(delta Delta) string {
	if len(delta.Changes) == 0 {
		return "no changes"
	}
	lines := []string{"Applied patch:"}
	for _, change := range delta.Changes {
		switch {
		case change.Kind == "update" && strings.TrimSpace(change.MoveTo) != "":
			lines = append(lines, fmt.Sprintf("- moved/updated %s -> %s", change.Path, change.MoveTo))
		default:
			lines = append(lines, fmt.Sprintf("- %s %s", change.Kind, change.Path))
		}
	}
	return strings.Join(lines, "\n")
}
