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
		{Source: "internal/chatcommands/build.go", Target: "internal/chatcommands/build.go"},
		{Source: "internal/chatcommands/chatcommands.go", Target: "internal/chatcommands/chatcommands.go"},
		{Source: "internal/chatcommands/execute.go", Target: "internal/chatcommands/execute.go"},
		{Source: "internal/chatcommands/types.go", Target: "internal/chatcommands/types.go"},
		{Source: "internal/hostbridge/types.go", Target: "internal/hostbridge/types.go"},
		{Source: "internal/hostbridge/client/client.go", Target: "internal/hostbridge/client/client.go"},
		{Source: "internal/hostbridge/server/allowed.go", Target: "internal/hostbridge/server/allowed.go"},
		{Source: "internal/hostbridge/server/listen.go", Target: "internal/hostbridge/server/listen.go"},
		{Source: "internal/hostbridge/server/runner.go", Target: "internal/hostbridge/server/runner.go"},
		{Source: "internal/hostbridge/server/server.go", Target: "internal/hostbridge/server/server.go"},
		{Source: "internal/hostbridgetls/tls.go", Target: "internal/hostbridgetls/tls.go"},
		{Source: "internal/modeluuid/coding.go", Target: "internal/modeluuid/coding.go"},
		{Source: "internal/modeluuid/uuid.go", Target: "internal/modeluuid/uuid.go"},
		{Source: "internal/messenger/messenger.go", Target: "internal/messenger/messenger.go"},
	}
}

func SelectedTargetsSummary() string {
	var parts []string
	for _, spec := range SelectedFiles() {
		parts = append(parts, spec.Target)
	}
	return strings.Join(parts, ", ")
}
