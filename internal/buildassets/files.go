package buildassets

import "strings"

type FileSpec struct {
	Source string
	Target string
}

func SelectedFiles() []FileSpec {
	return []FileSpec{
		{Source: "docker", Target: "."},
		{Source: "LICENSE", Target: "LICENSE"},
		{Source: "go.mod", Target: "go.mod"},
		{Source: "go.sum", Target: "go.sum"},
		{Source: "cmd/apply_patch", Target: "cmd/apply_patch"},
		{Source: "cmd/hostbridge", Target: "cmd/hostbridge"},
		{Source: "cmd/tools", Target: "cmd/tools"},
		{Source: "cmd/toolloop", Target: "cmd/toolloop"},
		{Source: "internal", Target: "internal"},
	}
}

func SelectedTargetsSummary() string {
	var parts []string
	for _, spec := range SelectedFiles() {
		parts = append(parts, spec.Target)
	}
	return strings.Join(parts, ", ")
}
