package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	beginPatchMarker   = "*** Begin Patch"
	endPatchMarker     = "*** End Patch"
	addFileMarker      = "*** Add File: "
	deleteFileMarker   = "*** Delete File: "
	updateFileMarker   = "*** Update File: "
	moveToMarker       = "*** Move to: "
	endOfFileMarker    = "*** End of File"
	emptyContextMarker = "@@"
	contextMarker      = "@@ "
)

type Patch struct {
	Operations []Operation
}

type Operation struct {
	Kind     OperationKind
	Path     string
	MovePath string
	Contents string
	Chunks   []UpdateChunk
}

type OperationKind string

const (
	OperationAdd    OperationKind = "add"
	OperationDelete OperationKind = "delete"
	OperationUpdate OperationKind = "update"
)

type UpdateChunk struct {
	Context    string
	HasContext bool
	OldLines   []string
	NewLines   []string
	EndOfFile  bool
}

type Applier struct {
	Root string
}

type AffectedPaths struct {
	Added    []string
	Modified []string
	Deleted  []string
}

func (a Applier) Apply(patchText string, stdout io.Writer) error {
	patch, err := ParsePatch(patchText)
	if err != nil {
		return err
	}
	if len(patch.Operations) == 0 {
		return fmt.Errorf("No files were modified.")
	}
	affected := AffectedPaths{}
	for _, op := range patch.Operations {
		if err := a.applyOperation(op, &affected); err != nil {
			return err
		}
	}
	return printSummary(stdout, affected)
}

func ParsePatch(patchText string) (Patch, error) {
	lines := splitPatchLines(patchText)
	lines, err := unwrapLenientHeredoc(lines)
	if err != nil {
		return Patch{}, err
	}
	if len(lines) < 2 {
		return Patch{}, fmt.Errorf("Invalid patch: The first line of the patch must be '*** Begin Patch'")
	}
	if strings.TrimSpace(lines[0]) != beginPatchMarker {
		return Patch{}, fmt.Errorf("Invalid patch: The first line of the patch must be '*** Begin Patch'")
	}
	if strings.TrimSpace(lines[len(lines)-1]) != endPatchMarker {
		return Patch{}, fmt.Errorf("Invalid patch: The last line of the patch must be '*** End Patch'")
	}

	body := lines[1 : len(lines)-1]
	var operations []Operation
	for lineIndex := 0; lineIndex < len(body); {
		if strings.TrimSpace(body[lineIndex]) == "" {
			lineIndex++
			continue
		}
		op, nextIndex, err := parseOperation(body, lineIndex)
		if err != nil {
			return Patch{}, err
		}
		operations = append(operations, op)
		lineIndex = nextIndex
	}
	if len(operations) == 0 {
		return Patch{}, fmt.Errorf("Invalid patch: no file operations")
	}
	return Patch{Operations: operations}, nil
}

func splitPatchLines(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := strings.Split(text, "\n")
	for i := range parts {
		parts[i] = strings.TrimSuffix(parts[i], "\r")
	}
	return parts
}

func unwrapLenientHeredoc(lines []string) ([]string, error) {
	if len(lines) >= 4 && isEOFHeredocStart(lines[0]) && strings.TrimSpace(lines[len(lines)-1]) == "EOF" {
		return lines[1 : len(lines)-1], nil
	}
	return lines, nil
}

func isEOFHeredocStart(line string) bool {
	line = strings.TrimSpace(line)
	return line == "<<EOF" || line == "<<'EOF'" || line == "<<\"EOF\""
}

func parseOperation(lines []string, start int) (Operation, int, error) {
	line := strings.TrimSpace(lines[start])
	if path, ok := strings.CutPrefix(line, addFileMarker); ok {
		return parseAddOperation(path, lines, start)
	}
	if path, ok := strings.CutPrefix(line, deleteFileMarker); ok {
		return Operation{Kind: OperationDelete, Path: strings.TrimSpace(path)}, start + 1, nil
	}
	if path, ok := strings.CutPrefix(line, updateFileMarker); ok {
		return parseUpdateOperation(path, lines, start)
	}
	return Operation{}, 0, fmt.Errorf("Invalid patch hunk on line %d: '%s' is not a valid hunk header", start+2, line)
}

func parseAddOperation(path string, lines []string, start int) (Operation, int, error) {
	var contents strings.Builder
	index := start + 1
	addLines := 0
	for index < len(lines) {
		line := lines[index]
		if strings.HasPrefix(line, "+") {
			contents.WriteString(line[1:])
			contents.WriteByte('\n')
			addLines++
			index++
			continue
		}
		break
	}
	if addLines == 0 {
		return Operation{}, 0, fmt.Errorf("Invalid patch hunk on line %d: Add file hunk for path '%s' is empty", start+2, strings.TrimSpace(path))
	}
	return Operation{Kind: OperationAdd, Path: strings.TrimSpace(path), Contents: contents.String()}, index, nil
}

func parseUpdateOperation(path string, lines []string, start int) (Operation, int, error) {
	op := Operation{Kind: OperationUpdate, Path: strings.TrimSpace(path)}
	index := start + 1
	if index < len(lines) {
		if movePath, ok := strings.CutPrefix(strings.TrimSpace(lines[index]), moveToMarker); ok {
			op.MovePath = strings.TrimSpace(movePath)
			index++
		}
	}
	for index < len(lines) {
		if strings.TrimSpace(lines[index]) == "" {
			index++
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "*** ") {
			break
		}
		chunk, nextIndex, err := parseUpdateChunk(lines, index)
		if err != nil {
			return Operation{}, 0, err
		}
		op.Chunks = append(op.Chunks, chunk)
		index = nextIndex
	}
	if len(op.Chunks) == 0 {
		return Operation{}, 0, fmt.Errorf("Invalid patch hunk on line %d: Update file hunk for path '%s' is empty", start+2, op.Path)
	}
	return op, index, nil
}

func parseUpdateChunk(lines []string, start int) (UpdateChunk, int, error) {
	chunk := UpdateChunk{}
	line := strings.TrimSpace(lines[start])
	index := start
	if line == emptyContextMarker {
		index++
	} else if context, ok := strings.CutPrefix(line, contextMarker); ok {
		chunk.Context = context
		chunk.HasContext = true
		index++
	} else {
		return UpdateChunk{}, 0, fmt.Errorf("Invalid patch hunk on line %d: expected update hunk to start with @@", start+2)
	}

	lineCount := 0
	for index < len(lines) {
		line := lines[index]
		switch {
		case strings.TrimSpace(line) == endOfFileMarker:
			if lineCount == 0 {
				return UpdateChunk{}, 0, fmt.Errorf("Invalid patch hunk on line %d: Update hunk does not contain any lines", index+2)
			}
			chunk.EndOfFile = true
			return chunk, index + 1, nil
		case line == "":
			chunk.OldLines = append(chunk.OldLines, "")
			chunk.NewLines = append(chunk.NewLines, "")
		case strings.HasPrefix(line, " "):
			chunk.OldLines = append(chunk.OldLines, line[1:])
			chunk.NewLines = append(chunk.NewLines, line[1:])
		case strings.HasPrefix(line, "-"):
			chunk.OldLines = append(chunk.OldLines, line[1:])
		case strings.HasPrefix(line, "+"):
			chunk.NewLines = append(chunk.NewLines, line[1:])
		default:
			if lineCount == 0 {
				return UpdateChunk{}, 0, fmt.Errorf("Invalid patch hunk on line %d: unexpected line found in update hunk: '%s'", index+2, line)
			}
			return chunk, index, nil
		}
		lineCount++
		index++
	}
	if lineCount == 0 {
		return UpdateChunk{}, 0, fmt.Errorf("Invalid patch hunk on line %d: Update hunk does not contain any lines", start+2)
	}
	return chunk, index, nil
}

func (a Applier) applyOperation(op Operation, affected *AffectedPaths) error {
	switch op.Kind {
	case OperationAdd:
		path, err := a.resolvePatchPath(op.Path)
		if err != nil {
			return err
		}
		if err := writeFileCreatingParents(path, []byte(op.Contents)); err != nil {
			return fmt.Errorf("Failed to write file %s: %w", path, err)
		}
		affected.Added = append(affected.Added, op.Path)
		return nil
	case OperationDelete:
		path, err := a.resolvePatchPath(op.Path)
		if err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("Failed to delete file %s: %w", path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("Failed to delete file %s: path is a directory", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("Failed to delete file %s: %w", path, err)
		}
		affected.Deleted = append(affected.Deleted, op.Path)
		return nil
	case OperationUpdate:
		return a.applyUpdateOperation(op, affected)
	default:
		return fmt.Errorf("unsupported patch operation: %s", op.Kind)
	}
}

func (a Applier) applyUpdateOperation(op Operation, affected *AffectedPaths) error {
	path, err := a.resolvePatchPath(op.Path)
	if err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Failed to read file to update %s: %w", path, err)
	}
	newContent, err := deriveNewContents(string(original), op.Path, op.Chunks)
	if err != nil {
		return err
	}
	if op.MovePath == "" {
		if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
			return fmt.Errorf("Failed to write file %s: %w", path, err)
		}
		affected.Modified = append(affected.Modified, op.Path)
		return nil
	}
	dest, err := a.resolvePatchPath(op.MovePath)
	if err != nil {
		return err
	}
	if samePath(path, dest) {
		if err := os.WriteFile(path, []byte(newContent), 0o644); err != nil {
			return fmt.Errorf("Failed to write file %s: %w", path, err)
		}
		affected.Modified = append(affected.Modified, op.Path)
		return nil
	}
	if err := writeFileCreatingParents(dest, []byte(newContent)); err != nil {
		return fmt.Errorf("Failed to write file %s: %w", dest, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("Failed to remove original %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("Failed to remove original %s: path is a directory", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("Failed to remove original %s: %w", path, err)
	}
	affected.Modified = append(affected.Modified, op.Path)
	return nil
}

func deriveNewContents(original string, displayPath string, chunks []UpdateChunk) (string, error) {
	originalLines := strings.Split(original, "\n")
	if len(originalLines) > 0 && originalLines[len(originalLines)-1] == "" {
		originalLines = originalLines[:len(originalLines)-1]
	}
	replacements, err := computeReplacements(originalLines, displayPath, chunks)
	if err != nil {
		return "", err
	}
	newLines := applyReplacements(originalLines, replacements)
	if len(newLines) == 0 || newLines[len(newLines)-1] != "" {
		newLines = append(newLines, "")
	}
	return strings.Join(newLines, "\n"), nil
}

type replacement struct {
	Start int
	OldN  int
	New   []string
}

func computeReplacements(originalLines []string, displayPath string, chunks []UpdateChunk) ([]replacement, error) {
	replacements := make([]replacement, 0, len(chunks))
	lineIndex := 0
	for _, chunk := range chunks {
		if chunk.HasContext {
			idx := seekSequence(originalLines, []string{chunk.Context}, lineIndex, false)
			if idx < 0 {
				return nil, fmt.Errorf("Failed to find context '%s' in %s", chunk.Context, displayPath)
			}
			lineIndex = idx + 1
		}
		if len(chunk.OldLines) == 0 {
			replacements = append(replacements, replacement{Start: len(originalLines), OldN: 0, New: append([]string(nil), chunk.NewLines...)})
			continue
		}
		pattern := chunk.OldLines
		newLines := chunk.NewLines
		found := seekSequence(originalLines, pattern, lineIndex, chunk.EndOfFile)
		if found < 0 && len(pattern) > 0 && pattern[len(pattern)-1] == "" {
			pattern = pattern[:len(pattern)-1]
			if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
				newLines = newLines[:len(newLines)-1]
			}
			found = seekSequence(originalLines, pattern, lineIndex, chunk.EndOfFile)
		}
		if found < 0 {
			return nil, fmt.Errorf("Failed to find expected lines in %s:\n%s", displayPath, strings.Join(chunk.OldLines, "\n"))
		}
		replacements = append(replacements, replacement{Start: found, OldN: len(pattern), New: append([]string(nil), newLines...)})
		lineIndex = found + len(pattern)
	}
	sort.SliceStable(replacements, func(i, j int) bool { return replacements[i].Start < replacements[j].Start })
	return replacements, nil
}

func applyReplacements(lines []string, replacements []replacement) []string {
	out := append([]string(nil), lines...)
	for i := len(replacements) - 1; i >= 0; i-- {
		repl := replacements[i]
		if repl.Start > len(out) {
			repl.Start = len(out)
		}
		end := repl.Start + repl.OldN
		if end > len(out) {
			end = len(out)
		}
		out = append(out[:repl.Start], append(append([]string(nil), repl.New...), out[end:]...)...)
	}
	return out
}

func seekSequence(lines []string, pattern []string, start int, eof bool) int {
	if len(pattern) == 0 {
		return start
	}
	if len(pattern) > len(lines) {
		return -1
	}
	searchStart := start
	if eof && len(lines) >= len(pattern) {
		searchStart = len(lines) - len(pattern)
	}
	if searchStart < 0 {
		searchStart = 0
	}
	if found := seekSequenceWith(lines, pattern, searchStart, func(a, b string) bool { return a == b }); found >= 0 {
		return found
	}
	if found := seekSequenceWith(lines, pattern, searchStart, func(a, b string) bool { return strings.TrimRight(a, " \t") == strings.TrimRight(b, " \t") }); found >= 0 {
		return found
	}
	return seekSequenceWith(lines, pattern, searchStart, func(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) })
}

func seekSequenceWith(lines []string, pattern []string, searchStart int, match func(string, string) bool) int {
	limit := len(lines) - len(pattern)
	for i := searchStart; i <= limit; i++ {
		ok := true
		for offset := range pattern {
			if !match(lines[i+offset], pattern[offset]) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func (a Applier) resolvePatchPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("empty patch path")
	}
	if strings.ContainsRune(path, '\x00') {
		return "", fmt.Errorf("unsafe patch path %q: contains NUL", path)
	}
	if strings.Contains(path, `\`) {
		return "", fmt.Errorf("unsafe patch path %q: backslashes are not allowed", path)
	}
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("unsafe patch path %q: absolute paths are not allowed", path)
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe patch path %q: path traversal is not allowed", path)
		}
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == string(filepath.Separator) {
		return "", fmt.Errorf("unsafe patch path %q", path)
	}
	for _, part := range strings.Split(clean, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("unsafe patch path %q: path traversal is not allowed", path)
		}
	}
	root := a.Root
	if root == "" {
		var err error
		root, err = os.Getwd()
		if err != nil {
			return "", err
		}
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(absRoot, clean)
	rel, err := filepath.Rel(absRoot, resolved)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe patch path %q: path traversal is not allowed", path)
	}
	return resolved, nil
}

func writeFileCreatingParents(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func samePath(lhs string, rhs string) bool {
	lhsAbs, lhsErr := filepath.Abs(lhs)
	rhsAbs, rhsErr := filepath.Abs(rhs)
	return lhsErr == nil && rhsErr == nil && lhsAbs == rhsAbs
}

func printSummary(stdout io.Writer, affected AffectedPaths) error {
	if _, err := fmt.Fprintln(stdout, "Success. Updated the following files:"); err != nil {
		return err
	}
	for _, path := range affected.Added {
		if _, err := fmt.Fprintf(stdout, "A %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range affected.Modified {
		if _, err := fmt.Fprintf(stdout, "M %s\n", path); err != nil {
			return err
		}
	}
	for _, path := range affected.Deleted {
		if _, err := fmt.Fprintf(stdout, "D %s\n", path); err != nil {
			return err
		}
	}
	return nil
}
