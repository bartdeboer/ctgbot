package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunAppliesAddFileFromArgument(t *testing.T) {
	dir := t.TempDir()
	patch := wrapPatch(`*** Add File: hello.txt
+Hello
+world`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "hello.txt", "Hello\nworld\n")
	assertContains(t, stdout, "A hello.txt")
}

func TestAddFileOverwritesExistingFileLikeCodex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "existing.txt", "original\n")
	patch := wrapPatch(`*** Add File: existing.txt
+new
+content`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "existing.txt", "new\ncontent\n")
	assertContains(t, stdout, "A existing.txt")
}

func TestRunReadsPatchFromStdin(t *testing.T) {
	dir := t.TempDir()
	patch := wrapPatch(`*** Add File: stdin.txt
+from stdin`)

	stdout, stderr, code := runPatch(t, dir, nil, patch)

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "stdin.txt", "from stdin\n")
	assertContains(t, stdout, "A stdin.txt")
}

func TestApplyDeleteFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "obsolete.txt", "remove me\n")
	patch := wrapPatch(`*** Delete File: obsolete.txt`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "obsolete.txt")); !os.IsNotExist(err) {
		t.Fatalf("obsolete.txt exists after delete, stat err = %v", err)
	}
	assertContains(t, stdout, "D obsolete.txt")
}

func TestApplyUpdateFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.txt", "alpha\nbeta\ngamma\n")
	patch := wrapPatch(`*** Update File: app.txt
@@
 alpha
-beta
+bravo
 gamma`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "app.txt", "alpha\nbravo\ngamma\n")
	assertContains(t, stdout, "M app.txt")
}

func TestApplyMoveFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "old/name.txt", "one\ntwo\n")
	patch := wrapPatch(`*** Update File: old/name.txt
*** Move to: new/name.txt
@@
 one
-two
+dos`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "new/name.txt", "one\ndos\n")
	if _, err := os.Stat(filepath.Join(dir, "old/name.txt")); !os.IsNotExist(err) {
		t.Fatalf("old/name.txt exists after move, stat err = %v", err)
	}
	assertContains(t, stdout, "M old/name.txt")
}

func TestMoveOverwritesExistingDestinationLikeCodex(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "source.txt", "one\ntwo\n")
	writeFile(t, dir, "dest.txt", "existing destination\n")
	patch := wrapPatch(`*** Update File: source.txt
*** Move to: dest.txt
@@
 one
-two
+updated`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "dest.txt", "one\nupdated\n")
	if _, err := os.Stat(filepath.Join(dir, "source.txt")); !os.IsNotExist(err) {
		t.Fatalf("source.txt exists after move, stat err = %v", err)
	}
	assertContains(t, stdout, "M source.txt")
}

func TestApplyMultipleHunks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "multi.txt", "one\ntwo\nthree\nfour\nfive\n")
	patch := wrapPatch(`*** Update File: multi.txt
@@
-two
+TWO
@@
-four
+FOUR`)

	_, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "multi.txt", "one\nTWO\nthree\nFOUR\nfive\n")
}

func TestApplyMultipleOperations(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "edit.txt", "before\n")
	writeFile(t, dir, "delete.txt", "gone\n")
	patch := wrapPatch(`*** Add File: add.txt
+new
*** Update File: edit.txt
@@
-before
+after
*** Delete File: delete.txt`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "add.txt", "new\n")
	assertFile(t, dir, "edit.txt", "after\n")
	if _, err := os.Stat(filepath.Join(dir, "delete.txt")); !os.IsNotExist(err) {
		t.Fatalf("delete.txt exists after delete, stat err = %v", err)
	}
	assertContains(t, stdout, "A add.txt")
	assertContains(t, stdout, "M edit.txt")
	assertContains(t, stdout, "D delete.txt")
}

func TestApplyEndOfFileHunk(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "eof.txt", "first\nlast\n")
	patch := wrapPatch(`*** Update File: eof.txt
@@
-last
+final
*** End of File`)

	_, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "eof.txt", "first\nfinal\n")
}

func TestAllowsAbsolutePathLikeCodex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absolute.txt")
	patch := wrapPatch(`*** Add File: ` + path + `
+absolute`)

	stdout, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	assertFile(t, dir, "absolute.txt", "absolute\n")
	assertContains(t, stdout, "A "+path)
}

func TestAllowsParentTraversalLikeCodex(t *testing.T) {
	parent := t.TempDir()
	workspace := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	patch := wrapPatch(`*** Add File: ../outside.txt
+outside`)

	stdout, stderr, code := runPatch(t, workspace, []string{patch}, "")

	if code != 0 {
		t.Fatalf("run exit = %d, stderr = %s", code, stderr)
	}
	body, err := os.ReadFile(filepath.Join(parent, "outside.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "outside\n" {
		t.Fatalf("outside.txt = %q", string(body))
	}
	assertContains(t, stdout, "A ../outside.txt")
}

func TestMalformedPatchFails(t *testing.T) {
	dir := t.TempDir()
	cases := []string{
		"*** Add File: missing-begin\n+bad\n*** End Patch\n",
		wrapPatch(`*** Add File: missing-plus
bad`),
		wrapPatch(`*** Update File: empty.txt`),
		wrapPatch(`*** Update File: bad.txt
@@
bad`),
	}
	for _, patch := range cases {
		_, stderr, code := runPatch(t, dir, []string{patch}, "")
		if code == 0 {
			t.Fatalf("expected malformed patch to fail: %s", patch)
		}
		if strings.TrimSpace(stderr) == "" {
			t.Fatalf("expected stderr for malformed patch: %s", patch)
		}
	}
}

func TestExpectedLinesMustMatch(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "app.txt", "alpha\nbeta\n")
	patch := wrapPatch(`*** Update File: app.txt
@@
-missing
+new`)

	_, stderr, code := runPatch(t, dir, []string{patch}, "")

	if code == 0 {
		t.Fatal("expected missing lines to fail")
	}
	assertContains(t, stderr, "Failed to find expected lines")
}

func runPatch(t *testing.T, cwd string, args []string, stdin string) (string, string, int) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(args, strings.NewReader(stdin), &stdout, &stderr, cwd)
	return stdout.String(), stderr.String(), code
}

func wrapPatch(body string) string {
	return "*** Begin Patch\n" + body + "\n*** End Patch\n"
}

func writeFile(t *testing.T, dir string, name string, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFile(t *testing.T, dir string, name string, want string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != want {
		t.Fatalf("%s contents = %q, want %q", name, string(body), want)
	}
}

func assertContains(t *testing.T, body string, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Fatalf("output = %q, want substring %q", body, want)
	}
}
