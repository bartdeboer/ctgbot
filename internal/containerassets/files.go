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
		{Source: "internal/durationparse/duration.go", Target: "internal/durationparse/duration.go"},
		{Source: "internal/hostbridge/controller.go", Target: "internal/hostbridge/controller.go"},
		{Source: "internal/hostbridge/protocol.go", Target: "internal/hostbridge/protocol.go"},
		{Source: "internal/hostbridge/runtime.go", Target: "internal/hostbridge/runtime.go"},
		{Source: "internal/hostbridge/client/client.go", Target: "internal/hostbridge/client/client.go"},
		{Source: "internal/hostbridge/protocol/protocol.go", Target: "internal/hostbridge/protocol/protocol.go"},
		{Source: "internal/hostbridge/server/server.go", Target: "internal/hostbridge/server/server.go"},
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
