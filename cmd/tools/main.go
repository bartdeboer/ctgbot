package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	defaultReadLimit = 2000
	maxPDFPages      = 20
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "read":
		err = runRead(args[1:], stdout)
	case "write":
		err = runWrite(args[1:], stdin, stdout)
	case "edit":
		err = runEdit(args[1:], stdout)
	default:
		printUsage(stderr)
		return 2
	}
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  tools read --file /abs/path [--offset N] [--limit N] [--pages 1-5]")
	fmt.Fprintln(w, "  tools write --file /abs/path [--content TEXT]")
	fmt.Fprintln(w, "  tools edit --file /abs/path --old TEXT --new TEXT [--replace-all]")
}

func runRead(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("tools read", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	filePath := fs.String("file", "", "absolute file path")
	limit := fs.Int("limit", defaultReadLimit, "number of lines to read")
	offset := fs.Int("offset", 0, "0-indexed line offset")
	pages := fs.String("pages", "", "PDF page range")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected read argument: %s", fs.Arg(0))
	}
	path, err := resolveAbsoluteFile(*filePath)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", path)
		}
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("path is a directory: %s", path)
	}
	if *limit <= 0 {
		return fmt.Errorf("limit must be greater than 0")
	}
	if *offset < 0 {
		return fmt.Errorf("offset must be greater than or equal to 0")
	}
	if err := validatePagesForPath(path, *pages); err != nil {
		return err
	}

	if isLikelyVisualFile(path) {
		if err := markRead(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "[Image file read: %s]\n", path)
		fmt.Fprintf(stdout, "[Visual rendering is not available in this CLI; size=%d bytes]\n", info.Size())
		return nil
	}
	if isPDF(path) {
		if strings.TrimSpace(*pages) == "" {
			return fmt.Errorf("pages is required for PDF files")
		}
		if err := markRead(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "[PDF file read: %s]\n", path)
		fmt.Fprintf(stdout, "[Page rendering is not available in this CLI; pages=%s size=%d bytes]\n", strings.TrimSpace(*pages), info.Size())
		return nil
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(body) == 0 {
		if err := markRead(path); err != nil {
			return err
		}
		fmt.Fprintln(stdout, "[System warning: file is empty]")
		return nil
	}
	if !utf8.Valid(body) {
		if err := markRead(path); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "[Binary file read: %s]\n", path)
		fmt.Fprintf(stdout, "[Text rendering is not available; size=%d bytes]\n", info.Size())
		return nil
	}
	if err := writeNumberedLines(stdout, string(body), *offset, *limit); err != nil {
		return err
	}
	return markRead(path)
}

func runWrite(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("tools write", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	filePath := fs.String("file", "", "absolute file path")
	content := fs.String("content", "", "full file content")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected write argument: %s", fs.Arg(0))
	}
	path, err := resolveAbsoluteFile(*filePath)
	if err != nil {
		return err
	}
	body := []byte(*content)
	if *content == "" {
		body, err = io.ReadAll(stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	}
	if fileExists(path) {
		if err := requireRead(path); err != nil {
			return err
		}
	}
	if err := parentDirectoryExists(path); err != nil {
		return err
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return err
	}
	if err := markRead(path); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote: %s\n", path)
	return nil
}

func runEdit(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("tools edit", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	filePath := fs.String("file", "", "absolute file path")
	oldString := fs.String("old", "", "exact string to replace")
	newString := fs.String("new", "", "replacement string")
	replaceAll := fs.Bool("replace-all", false, "replace every occurrence")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected edit argument: %s", fs.Arg(0))
	}
	path, err := resolveAbsoluteFile(*filePath)
	if err != nil {
		return err
	}
	if err := requireRead(path); err != nil {
		return err
	}
	if *oldString == *newString {
		return fmt.Errorf("old and new strings must be different")
	}
	if *oldString == "" {
		return fmt.Errorf("old string must not be empty")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(body)
	count := strings.Count(text, *oldString)
	if count == 0 {
		return fmt.Errorf("old string not found")
	}
	if !*replaceAll && count > 1 {
		return fmt.Errorf("old string appears %d times; include more context or use --replace-all", count)
	}
	n := 1
	if *replaceAll {
		n = -1
	}
	updated := strings.Replace(text, *oldString, *newString, n)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "edited: %s\n", path)
	fmt.Fprintf(stdout, "replacements: %d\n", countReplacements(count, *replaceAll))
	return nil
}

func resolveAbsoluteFile(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("missing file path")
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("file path must be absolute: %s", raw)
	}
	clean := filepath.Clean(raw)
	if clean == string(filepath.Separator) {
		return "", fmt.Errorf("file path must identify a file")
	}
	return clean, nil
}

func writeNumberedLines(w io.Writer, text string, offset int, limit int) error {
	lines := strings.Split(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}
	for i := offset; i < end; i++ {
		if _, err := fmt.Fprintf(w, "%6d\t%s\n", i+1, lines[i]); err != nil {
			return err
		}
	}
	return nil
}

func validatePagesForPath(path string, pages string) error {
	pages = strings.TrimSpace(pages)
	if pages == "" {
		return nil
	}
	if !isPDF(path) {
		return fmt.Errorf("pages is only supported for PDF files")
	}
	count, err := pageRangeCount(pages)
	if err != nil {
		return err
	}
	if count > maxPDFPages {
		return fmt.Errorf("pages range exceeds max %d pages", maxPDFPages)
	}
	return nil
}

func pageRangeCount(pages string) (int, error) {
	parts := strings.Split(pages, "-")
	switch len(parts) {
	case 1:
		page, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || page <= 0 {
			return 0, fmt.Errorf("invalid pages value: %s", pages)
		}
		return 1, nil
	case 2:
		start, startErr := strconv.Atoi(strings.TrimSpace(parts[0]))
		end, endErr := strconv.Atoi(strings.TrimSpace(parts[1]))
		if startErr != nil || endErr != nil || start <= 0 || end < start {
			return 0, fmt.Errorf("invalid pages value: %s", pages)
		}
		return end - start + 1, nil
	default:
		return 0, fmt.Errorf("invalid pages value: %s", pages)
	}
}

func isPDF(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".pdf")
}

func isLikelyVisualFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff":
		return true
	default:
		return false
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func parentDirectoryExists(path string) error {
	parent := filepath.Dir(path)
	info, err := os.Stat(parent)
	if err != nil {
		return fmt.Errorf("parent directory does not exist: %s", parent)
	}
	if !info.IsDir() {
		return fmt.Errorf("parent path is not a directory: %s", parent)
	}
	return nil
}

func countReplacements(count int, replaceAll bool) int {
	if replaceAll {
		return count
	}
	return 1
}

type readState struct {
	ReadFiles []string `json:"read_files"`
}

func markRead(path string) error {
	path, err := canonicalPath(path)
	if err != nil {
		return err
	}
	state, err := loadReadState()
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, existing := range state.ReadFiles {
		seen[existing] = true
	}
	if !seen[path] {
		state.ReadFiles = append(state.ReadFiles, path)
		sort.Strings(state.ReadFiles)
	}
	return saveReadState(state)
}

func requireRead(path string) error {
	path, err := canonicalPath(path)
	if err != nil {
		return err
	}
	state, err := loadReadState()
	if err != nil {
		return err
	}
	for _, existing := range state.ReadFiles {
		if existing == path {
			return nil
		}
	}
	return fmt.Errorf("file must be read before edit/write: %s", path)
}

func canonicalPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func loadReadState() (readState, error) {
	path, err := readStatePath()
	if err != nil {
		return readState{}, err
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return readState{}, nil
		}
		return readState{}, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return readState{}, nil
	}
	var state readState
	if err := json.Unmarshal(body, &state); err != nil {
		return readState{}, err
	}
	return state, nil
}

func saveReadState(state readState) error {
	path, err := readStatePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(body, '\n'), 0o600)
}

func readStatePath() (string, error) {
	if explicit := strings.TrimSpace(os.Getenv("CTGBOT_TOOLS_STATE")); explicit != "" {
		if !filepath.IsAbs(explicit) {
			return "", fmt.Errorf("CTGBOT_TOOLS_STATE must be absolute")
		}
		return filepath.Clean(explicit), nil
	}
	stateDir := strings.TrimSpace(os.Getenv("CTGBOT_TOOLS_STATE_DIR"))
	if stateDir == "" {
		stateDir = filepath.Join(os.TempDir(), "ctgbot-tools")
	}
	if !filepath.IsAbs(stateDir) {
		return "", fmt.Errorf("CTGBOT_TOOLS_STATE_DIR must be absolute")
	}
	session := strings.TrimSpace(os.Getenv("CTGBOT_TOOLS_SESSION"))
	if session == "" {
		session = "default"
	}
	return filepath.Join(filepath.Clean(stateDir), safeStateFilename(session)+".json"), nil
}

func safeStateFilename(session string) string {
	var b strings.Builder
	for _, r := range session {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := strings.Trim(b.String(), ".")
	if name == "" {
		return "default"
	}
	return name
}
