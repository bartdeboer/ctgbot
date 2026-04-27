package containerassets

import "strings"

type FileSpec struct {
	Source string
	Target string
}

func SelectedFiles() []FileSpec {
	return []FileSpec{
		{Source: "docker/Dockerfile", Target: "Dockerfile"},
		{Source: "docker/Dockerfile.agent", Target: "Dockerfile.agent"},
		{Source: "docker/Dockerfile.agent-gpu", Target: "Dockerfile.agent-gpu"},
		{Source: "go.mod", Target: "go.mod"},
		{Source: "go.sum", Target: "go.sum"},
		{Source: "cmd/hostbridge/main.go", Target: "cmd/hostbridge/main.go"},
		{Source: "internal/commandengine/engine.go", Target: "internal/commandengine/engine.go"},
		{Source: "internal/commandengine/registry.go", Target: "internal/commandengine/registry.go"},
		{Source: "internal/commandengine/router.go", Target: "internal/commandengine/router.go"},
		{Source: "internal/commandengine/types.go", Target: "internal/commandengine/types.go"},
		{Source: "internal/configengine/manager.go", Target: "internal/configengine/manager.go"},
		{Source: "internal/configengine/schema.go", Target: "internal/configengine/schema.go"},
		{Source: "internal/hostbridge/types.go", Target: "internal/hostbridge/types.go"},
		{Source: "internal/hostbridge/client/client.go", Target: "internal/hostbridge/client/client.go"},
		{Source: "internal/hostbridge/client/command.go", Target: "internal/hostbridge/client/command.go"},
		{Source: "internal/hostbridgetls/tls.go", Target: "internal/hostbridgetls/tls.go"},
		{Source: "internal/modeluuid/coding.go", Target: "internal/modeluuid/coding.go"},
		{Source: "internal/modeluuid/uuid.go", Target: "internal/modeluuid/uuid.go"},
		{Source: "internal/schema/commands/config.go", Target: "internal/schema/commands/config.go"},
		{Source: "internal/schema/commands/example.go", Target: "internal/schema/commands/example.go"},
		{Source: "internal/schema/commands/hostbridge.go", Target: "internal/schema/commands/hostbridge.go"},
		{Source: "internal/schema/commands/thread.go", Target: "internal/schema/commands/thread.go"},
		{Source: "internal/schema/routers/config.go", Target: "internal/schema/routers/config.go"},
		{Source: "internal/schema/routers/definitions.go", Target: "internal/schema/routers/definitions.go"},
		{Source: "internal/schema/routers/hostbridge.go", Target: "internal/schema/routers/hostbridge.go"},
		{Source: "internal/schema/routers/message.go", Target: "internal/schema/routers/message.go"},
		{Source: "internal/schema/routers/thread.go", Target: "internal/schema/routers/thread.go"},
		{Source: "internal/simplerbac/evaluate.go", Target: "internal/simplerbac/evaluate.go"},
		{Source: "internal/simplerbac/policy.go", Target: "internal/simplerbac/policy.go"},
	}
}

func SelectedTargetsSummary() string {
	var parts []string
	for _, spec := range SelectedFiles() {
		parts = append(parts, spec.Target)
	}
	return strings.Join(parts, ", ")
}
