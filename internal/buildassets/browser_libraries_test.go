package buildassets

import (
	"archive/tar"
	"io"
	"strings"
	"testing"
)

// Check the actual packaged recipes, not a parallel list of build inputs.
func TestCodexBrowserLibraryRecipesInContext(t *testing.T) {
	rc, err := tarFromModule()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	want := map[string]string{
		"codex.base.Dockerfile": "FROM ctgbot-go-node-python-base:latest",
		"cuda.base.Dockerfile":  "FROM ctgbot-go-node-python-cuda-base:latest",
	}
	packages := strings.Fields("libnspr4 libnss3 libatk1.0-0 libatk-bridge2.0-0 libdbus-1-3 libcups2 libxcb1 libxkbcommon0 libasound2 libgbm1 libx11-6 libxext6 libcairo2 libpango-1.0-0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libatspi2.0-0")
	tr := tar.NewReader(rc)
	var blocks []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		parent, ok := want[h.Name]
		if !ok {
			continue
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		if !strings.HasPrefix(body, parent+"\n") {
			t.Fatalf("%s parent changed", h.Name)
		}
		start := strings.Index(body, "RUN apt-get update")
		end := strings.Index(body, "ARG CODEX_VERSION")
		if start < 0 || end < start {
			t.Fatalf("%s missing independent library layer", h.Name)
		}
		block := body[start:end]
		for _, p := range packages {
			if !strings.Contains(block, "        "+p+" \\\n") {
				t.Fatalf("%s missing package %s", h.Name, p)
			}
		}
		for _, s := range []string{"--no-install-recommends", "&& apt-get clean", "&& rm -rf /var/lib/apt/lists/*"} {
			if !strings.Contains(block, s) {
				t.Fatalf("%s missing %s", h.Name, s)
			}
		}
		for _, s := range []string{"chromium", "playwright", "--no-sandbox", "npm ", "curl "} {
			if strings.Contains(block, s) {
				t.Fatalf("%s unexpected browser install/runtime directive: %s", h.Name, s)
			}
		}
		blocks = append(blocks, block)
		delete(want, h.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing packaged recipes: %v", want)
	}
	if blocks[0] != blocks[1] {
		t.Fatal("standard/GPU package blocks diverged")
	}
}
