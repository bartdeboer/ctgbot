package commandengine

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/bartdeboer/go-clir"
)

func writeHelpLines(w io.Writer, routes []clir.RouteInfo, source Source) {
	for _, route := range routes {
		line := NormalizePattern(route.Pattern)
		if source == SourceMessage {
			line = "/" + line
		}
		if description := strings.TrimSpace(route.Description); description != "" {
			line += " - " + description
		}
		fmt.Fprintln(w, line)
	}
}

func writeHelpCompactLines(w io.Writer, routes []clir.RouteInfo, source Source) {
	writeHelpLines(w, compactHelpRoutes(routes), source)
}

func compactHelpRoutes(routes []clir.RouteInfo) []clir.RouteInfo {
	normalized := dedupeHelpRoutes(routes)
	type group struct {
		prefix      string
		description string
		children    map[string]struct{}
	}
	groups := map[string]*group{}
	var order []string
	for _, route := range normalized {
		parts := strings.Fields(NormalizePattern(route.Pattern))
		if len(parts) == 0 || isHelpParam(parts[0]) {
			continue
		}
		prefix := parts[0]
		g := groups[prefix]
		if g == nil {
			g = &group{prefix: prefix, children: map[string]struct{}{}}
			groups[prefix] = g
			order = append(order, prefix)
		}
		if len(parts) == 1 {
			if strings.TrimSpace(route.Description) != "" {
				g.description = strings.TrimSpace(route.Description)
			}
			continue
		}
		g.children[parts[1]] = struct{}{}
		if len(parts) == 2 && parts[1] == "help" && strings.TrimSpace(route.Description) != "" && g.description == "" {
			g.description = strings.TrimSpace(route.Description)
		}
	}

	grouped := map[string]clir.RouteInfo{}
	for _, prefix := range order {
		g := groups[prefix]
		if g == nil || len(g.children) < 2 {
			continue
		}
		g.children["help"] = struct{}{}
		children := make([]string, 0, len(g.children))
		for child := range g.children {
			children = append(children, child)
		}
		sort.SliceStable(children, func(i, j int) bool {
			if children[i] == "help" {
				return false
			}
			if children[j] == "help" {
				return true
			}
			return children[i] < children[j]
		})
		desc := strings.TrimSpace(g.description)
		if desc == "" {
			desc = groupHelpDescription(prefix)
		}
		grouped[prefix] = clir.RouteInfo{Pattern: prefix + " [ " + strings.Join(children, " | ") + " ]", Description: desc}
	}

	out := make([]clir.RouteInfo, 0, len(normalized))
	emitted := map[string]struct{}{}
	for _, route := range normalized {
		parts := strings.Fields(NormalizePattern(route.Pattern))
		if len(parts) >= 2 {
			if compact, ok := grouped[parts[0]]; ok {
				if _, seen := emitted[parts[0]]; !seen {
					out = append(out, compact)
					emitted[parts[0]] = struct{}{}
				}
				continue
			}
		}
		out = append(out, route)
	}
	return out
}

func dedupeHelpRoutes(routes []clir.RouteInfo) []clir.RouteInfo {
	byPattern := map[string]clir.RouteInfo{}
	var order []string
	for _, route := range routes {
		route.Pattern = NormalizePattern(route.Pattern)
		if route.Pattern == "" {
			continue
		}
		if existing, ok := byPattern[route.Pattern]; ok {
			if strings.TrimSpace(existing.Description) == "" && strings.TrimSpace(route.Description) != "" {
				byPattern[route.Pattern] = route
			}
			continue
		}
		byPattern[route.Pattern] = route
		order = append(order, route.Pattern)
	}
	out := make([]clir.RouteInfo, 0, len(order))
	for _, pattern := range order {
		out = append(out, byPattern[pattern])
	}
	return out
}

func isHelpParam(part string) bool {
	return strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">")
}

func groupHelpDescription(pattern string) string {
	parts := strings.Fields(NormalizePattern(pattern))
	if len(parts) == 0 {
		return "Commands"
	}
	group := parts[0]
	if strings.Contains(group, "/") {
		group = strings.TrimSpace(group)
		return group + " commands"
	}
	switch group {
	case "codex":
		return "Codex commands"
	case "claude":
		return "Claude commands"
	case "config":
		return "Global config commands"
	case "component":
		return "Component commands"
	case "dropped":
		return "Dropped-message commands"
	case "llamacpp":
		return "llama.cpp commands"
	case "model":
		return "AI model commands"
	case "process":
		return "Process commands"
	case "semantic":
		return "Semantic search commands"
	case "thread":
		return "Thread commands"
	case "turn":
		return "Current turn commands"
	default:
		return group + " commands"
	}
}
