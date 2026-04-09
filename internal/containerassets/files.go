package containerassets

import "strings"

type FileSpec struct {
	Source string
	Target string
}

func SelectedFiles() []FileSpec {
	return []FileSpec{
		{Source: "docker/Dockerfile", Target: "Dockerfile"},
		{Source: "go.mod", Target: "go.mod"},
		{Source: "go.sum", Target: "go.sum"},
		{Source: "cmd/hostbridge/main.go", Target: "cmd/hostbridge/main.go"},
		{Source: "internal/hostbridge/protocol.go", Target: "internal/hostbridge/protocol.go"},
		{Source: "internal/hostbridgetls/tls.go", Target: "internal/hostbridgetls/tls.go"},
	}
}

func SelectedTargetsSummary() string {
	var parts []string
	for _, spec := range SelectedFiles() {
		parts = append(parts, spec.Target)
	}
	return strings.Join(parts, ", ")
}
