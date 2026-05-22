package buildassets

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildContextTarContainsDockerfiles(t *testing.T) {
	rc, err := tarFromModule()
	if err != nil {
		t.Fatalf("tarFromModule: %v", err)
	}
	defer rc.Close()

	found := tarNames(t, rc)

	for _, name := range []string{
		"codex.base.Dockerfile",
		"codex.Dockerfile",
		"go-node-python.base.Dockerfile",
		"go-node-python-cuda.base.Dockerfile",
		"cuda.Dockerfile",
		"claude.base.Dockerfile",
		"claude.Dockerfile",
		"LICENSE",
		"cmd/apply_patch/main.go",
		"cmd/hostbridge/main.go",
		"cmd/toolloop/main.go",
		"internal/commandset/commandset.go",
	} {
		if !found[name] {
			t.Fatalf("build context tar does not contain %s", name)
		}
	}
}

func TestBuildContextTarIncludesGeneratedVersionButSkipsGeneratedTarball(t *testing.T) {
	versionPath, err := filepathFromModule("internal/buildassets/assets/version.txt")
	if err != nil {
		t.Fatalf("version path: %v", err)
	}
	if err := os.WriteFile(versionPath, []byte("v0.1.0-test\n"), 0o644); err != nil {
		t.Fatalf("write version: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(versionPath) })

	rc, err := tarFromModule()
	if err != nil {
		t.Fatalf("tarFromModule: %v", err)
	}
	defer rc.Close()
	found := tarNames(t, rc)
	if !found["internal/buildassets/assets/version.txt"] {
		t.Fatalf("build context tar does not contain generated version.txt")
	}
	if found["internal/buildassets/assets/src.tar.gz"] {
		t.Fatalf("build context tar should not contain generated src.tar.gz")
	}
}

func tarNames(t *testing.T, rc io.Reader) map[string]bool {
	t.Helper()
	tr := tar.NewReader(rc)
	found := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("read tar header: %v", err)
			}
			break
		}
		found[hdr.Name] = true
	}
	return found
}

func filepathFromModule(name string) (string, error) {
	root, err := moduleRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(name)), nil
}
