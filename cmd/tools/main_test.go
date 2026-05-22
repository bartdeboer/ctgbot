package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPrintsCatNAndSupportsOffsetLimit(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	t.Setenv("CTGBOT_TOOLS_STATE", state)
	path := filepath.Join(dir, "notes.txt")
	writeTestFile(t, path, "one\ntwo\nthree\n")

	stdout, stderr, code := runTools(t, []string{"read", "--file", path, "--offset", "1", "--limit", "2"}, "")

	if code != 0 {
		t.Fatalf("read exit = %d, stderr = %s", code, stderr)
	}
	if stdout != "     2\ttwo\n     3\tthree\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	assertContains(t, readTestFile(t, state), path)
}

func TestReadEmptyFileWarning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "empty.txt")
	writeTestFile(t, path, "")

	stdout, stderr, code := runTools(t, []string{"read", "--file", path}, "")

	if code != 0 {
		t.Fatalf("read exit = %d, stderr = %s", code, stderr)
	}
	assertContains(t, stdout, "file is empty")
}

func TestReadRejectsRelativePath(t *testing.T) {
	_, stderr, code := runTools(t, []string{"read", "--file", "relative.txt"}, "")
	if code == 0 {
		t.Fatal("expected relative path to fail")
	}
	assertContains(t, stderr, "must be absolute")
}

func TestReadPDFRequiresPagesAndValidatesPageLimit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "doc.pdf")
	writeTestFile(t, path, "%PDF fake")

	_, stderr, code := runTools(t, []string{"read", "--file", path}, "")
	if code == 0 {
		t.Fatal("expected missing pages to fail")
	}
	assertContains(t, stderr, "pages is required")

	_, stderr, code = runTools(t, []string{"read", "--file", path, "--pages", "1-21"}, "")
	if code == 0 {
		t.Fatal("expected too many pages to fail")
	}
	assertContains(t, stderr, "max 20")
}

func TestWriteCreatesNewFileWithoutReadButRequiresParent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "new.txt")

	stdout, stderr, code := runTools(t, []string{"write", "--file", path, "--content", "hello\n"}, "")

	if code != 0 {
		t.Fatalf("write exit = %d, stderr = %s", code, stderr)
	}
	assertContains(t, stdout, "wrote:")
	if got := readTestFile(t, path); got != "hello\n" {
		t.Fatalf("new file = %q", got)
	}

	missingParent := filepath.Join(dir, "missing", "file.txt")
	_, stderr, code = runTools(t, []string{"write", "--file", missingParent, "--content", "x"}, "")
	if code == 0 {
		t.Fatal("expected missing parent to fail")
	}
	assertContains(t, stderr, "parent directory does not exist")
}

func TestWriteMarksFileReadForImmediateEdit(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "new.txt")

	_, stderr, code := runTools(t, []string{"write", "--file", path, "--content", "hello"}, "")
	if code != 0 {
		t.Fatalf("write failed: %s", stderr)
	}
	_, stderr, code = runTools(t, []string{"edit", "--file", path, "--old", "hello", "--new", "world"}, "")
	if code != 0 {
		t.Fatalf("edit after write failed: %s", stderr)
	}
	if got := readTestFile(t, path); got != "world" {
		t.Fatalf("file = %q", got)
	}
}

func TestWriteExistingRequiresRead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "existing.txt")
	writeTestFile(t, path, "old\n")

	_, stderr, code := runTools(t, []string{"write", "--file", path, "--content", "new\n"}, "")
	if code == 0 {
		t.Fatal("expected write existing before read to fail")
	}
	assertContains(t, stderr, "must be read before edit/write")

	_, stderr, code = runTools(t, []string{"read", "--file", path}, "")
	if code != 0 {
		t.Fatalf("read before write failed: %s", stderr)
	}
	_, stderr, code = runTools(t, []string{"write", "--file", path}, "from stdin\n")
	if code != 0 {
		t.Fatalf("write after read failed: %s", stderr)
	}
	if got := readTestFile(t, path); got != "from stdin\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestEditRequiresReadAndReplacesUniqueString(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "edit.txt")
	writeTestFile(t, path, "hello world\n")

	_, stderr, code := runTools(t, []string{"edit", "--file", path, "--old", "world", "--new", "ctgbot"}, "")
	if code == 0 {
		t.Fatal("expected edit before read to fail")
	}
	assertContains(t, stderr, "must be read before edit/write")

	_, stderr, code = runTools(t, []string{"read", "--file", path}, "")
	if code != 0 {
		t.Fatalf("read before edit failed: %s", stderr)
	}
	stdout, stderr, code := runTools(t, []string{"edit", "--file", path, "--old", "world", "--new", "ctgbot"}, "")
	if code != 0 {
		t.Fatalf("edit failed: %s", stderr)
	}
	assertContains(t, stdout, "replacements: 1")
	if got := readTestFile(t, path); got != "hello ctgbot\n" {
		t.Fatalf("file = %q", got)
	}
}

func TestEditRejectsNonUniqueUnlessReplaceAll(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CTGBOT_TOOLS_STATE", filepath.Join(dir, "state.json"))
	path := filepath.Join(dir, "edit.txt")
	writeTestFile(t, path, "x x x")
	_, stderr, code := runTools(t, []string{"read", "--file", path}, "")
	if code != 0 {
		t.Fatalf("read failed: %s", stderr)
	}

	_, stderr, code = runTools(t, []string{"edit", "--file", path, "--old", "x", "--new", "y"}, "")
	if code == 0 {
		t.Fatal("expected non-unique edit to fail")
	}
	assertContains(t, stderr, "appears 3 times")

	_, stderr, code = runTools(t, []string{"edit", "--file", path, "--old", "x", "--new", "y", "--replace-all"}, "")
	if code != 0 {
		t.Fatalf("replace all failed: %s", stderr)
	}
	if got := readTestFile(t, path); got != "y y y" {
		t.Fatalf("file = %q", got)
	}
}

func runTools(t *testing.T, args []string, stdin string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, strings.NewReader(stdin), &stdout, &stderr)
	return stdout.String(), stderr.String(), code
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func assertContains(t *testing.T, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("body = %q, want substring %q", body, want)
	}
}
