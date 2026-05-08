package imageassets

import (
	"archive/tar"
	"io"
	"testing"
)

func TestBuildContextTarContainsDockerfiles(t *testing.T) {
	rc, err := tarFromModule()
	if err != nil {
		t.Fatalf("tarFromModule: %v", err)
	}
	defer rc.Close()

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

	for _, name := range []string{
		"Dockerfile",
		"slim.Dockerfile",
		"cuda.Dockerfile",
		"cmd/hostbridge/main.go",
		"internal/commandset/commandset.go",
	} {
		if !found[name] {
			t.Fatalf("build context tar does not contain %s", name)
		}
	}
}
