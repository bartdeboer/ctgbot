package containerassets

import (
	"archive/tar"
	"io"
	"testing"
)

func TestBuildContextTarContainsDockerfile(t *testing.T) {
	rc, err := BuildContextTar()
	if err != nil {
		t.Fatalf("BuildContextTar: %v", err)
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	foundDockerfile := false
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err != io.EOF {
				t.Fatalf("read tar header: %v", err)
			}
			break
		}
		if hdr.Name == "Dockerfile" {
			foundDockerfile = true
			break
		}
	}

	if !foundDockerfile {
		t.Fatalf("build context tar does not contain Dockerfile")
	}
}
