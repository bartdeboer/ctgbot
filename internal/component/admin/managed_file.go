package admin

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
)

func formatManagedFileList(files []component.ManagedFile) string {
	if len(files) == 0 {
		return "no managed files"
	}
	lines := make([]string, 0, len(files))
	for _, file := range sortedManagedFiles(files) {
		rel, err := safeManagedRelativePath(file.RelativePath)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s\tinvalid\t%s\t%s", file.RelativePath, requiredLabel(file.Required), sensitiveLabel(file.Sensitive)))
			continue
		}
		file.RelativePath = rel
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s", file.RelativePath, requiredLabel(file.Required), sensitiveLabel(file.Sensitive)))
	}
	return strings.Join(lines, "\n")
}

func formatManagedFileStatus(home string, files []component.ManagedFile) string {
	if len(files) == 0 {
		return "no managed files"
	}
	lines := make([]string, 0, len(files))
	for _, file := range sortedManagedFiles(files) {
		rel, err := safeManagedRelativePath(file.RelativePath)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s\tinvalid\t%s\t%s", file.RelativePath, requiredLabel(file.Required), sensitiveLabel(file.Sensitive)))
			continue
		}
		file.RelativePath = rel
		target, _, err := managedFileTarget(home, file.RelativePath)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s\tinvalid\t%s\t%s", file.RelativePath, requiredLabel(file.Required), sensitiveLabel(file.Sensitive)))
			continue
		}
		state := "missing"
		if _, err := os.Stat(target); err == nil {
			state = "present"
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s", file.RelativePath, state, requiredLabel(file.Required), sensitiveLabel(file.Sensitive)))
	}
	return strings.Join(lines, "\n")
}

func sortedManagedFiles(files []component.ManagedFile) []component.ManagedFile {
	out := append([]component.ManagedFile(nil), files...)
	sort.Slice(out, func(i, j int) bool { return out[i].RelativePath < out[j].RelativePath })
	return out
}

func declaredManagedFile(files []component.ManagedFile, requested string) (component.ManagedFile, error) {
	requested, err := safeManagedRelativePath(requested)
	if err != nil {
		return component.ManagedFile{}, err
	}
	for _, file := range files {
		declared, err := safeManagedRelativePath(file.RelativePath)
		if err != nil {
			continue
		}
		if requested == declared {
			file.RelativePath = declared
			return file, nil
		}
	}
	return component.ManagedFile{}, fmt.Errorf("managed file is not declared: %s", requested)
}

func safeManagedRelativePath(value string) (string, error) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || value == "." {
		return "", fmt.Errorf("missing managed file")
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("managed file must be relative: %s", value)
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid managed file path: %s", value)
		}
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean != value {
		return "", fmt.Errorf("invalid managed file path: %s", value)
	}
	return value, nil
}

func writeManagedFile(home string, file component.ManagedFile, content []byte) error {
	target, parent, err := managedFileTarget(home, file.RelativePath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	if err := ensureManagedParentInside(home, parent); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if file.Sensitive {
		mode = 0o600
	}
	tmp, err := os.CreateTemp(parent, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	return os.Chmod(target, mode)
}

func managedFileTarget(home string, relativePath string) (target string, parent string, err error) {
	home = strings.TrimSpace(home)
	if home == "" {
		return "", "", fmt.Errorf("missing component home")
	}
	rel, err := safeManagedRelativePath(relativePath)
	if err != nil {
		return "", "", err
	}
	absHome, err := filepath.Abs(home)
	if err != nil {
		return "", "", err
	}
	target = filepath.Join(absHome, filepath.FromSlash(rel))
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", "", err
	}
	if !pathWithin(absHome, absTarget) {
		return "", "", fmt.Errorf("managed file target escapes component home: %s", rel)
	}
	return absTarget, filepath.Dir(absTarget), nil
}

func ensureManagedParentInside(home string, parent string) error {
	absHome, err := filepath.Abs(strings.TrimSpace(home))
	if err != nil {
		return err
	}
	realHome, err := filepath.EvalSymlinks(absHome)
	if err != nil {
		return err
	}
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return err
	}
	if !pathWithin(realHome, realParent) {
		return fmt.Errorf("managed file parent escapes component home: %s", parent)
	}
	return nil
}

func pathWithin(parent string, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func requiredLabel(required bool) string {
	if required {
		return "required"
	}
	return "optional"
}

func sensitiveLabel(sensitive bool) string {
	if sensitive {
		return "sensitive"
	}
	return "non-sensitive"
}
