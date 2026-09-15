package buildassets

import "testing"

func TestCopilotRecipesInGeneratedSourceContext(t *testing.T) {
	rc, err := tarFromModule()
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	names := tarNames(t, rc)
	for _, name := range []string{"copilot.Dockerfile", "copilot.base.Dockerfile", "go-node-python.base.Dockerfile", "internal/component/copilot/component.go"} {
		if !names[name] {
			t.Fatal("missing embedded source entry", name)
		}
	}
}
