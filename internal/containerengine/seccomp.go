package containerengine

import (
	"fmt"
	"strings"
)

func SeccompSecurityOpts(mode string) ([]string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "default", "docker-default":
		return nil, nil
	case "unconfined":
		return []string{"seccomp=unconfined"}, nil
	default:
		return nil, fmt.Errorf("unsupported docker seccomp mode %q", mode)
	}
}
