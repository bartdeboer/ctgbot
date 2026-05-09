package admin

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/go-clir"
)

func FormatHelp(definitions []commandengine.Definition) string {
	var lines []string
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			line := commandengine.NormalizePattern(route.Pattern)
			if help := strings.TrimSpace(definition.Help); help != "" {
				line += " - " + help
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "no component commands"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func definitionsForSource(definitions []commandengine.Definition, source commandengine.Source) []commandengine.Definition {
	if source == "" {
		return definitions
	}
	out := make([]commandengine.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.AllowsSource(source) {
			out = append(out, definition)
		}
	}
	return out
}

func buildComponentHelp(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	return ComponentHelpCommand{Component: componentRef}, nil
}

func buildAuthStatus(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
		return nil, fmt.Errorf("unexpected auth status arguments: %s", extra)
	}
	return AuthStatusCommand{Component: componentRef}, nil
}

func buildManagedFileList(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	return ManagedFileListCommand{Component: componentRef}, nil
}

func buildManagedFileStatus(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	return ManagedFileStatusCommand{Component: componentRef}, nil
}

func buildManagedFilePut(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	file := strings.TrimSpace(req.Params["file"])
	if file == "" {
		return nil, fmt.Errorf("missing managed file")
	}
	fs := flag.NewFlagSet("component managed-file put", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contentType := fs.String("type", "", "Optional content type")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return ManagedFilePutCommand{
		Component:   componentRef,
		File:        file,
		ContentType: strings.TrimSpace(*contentType),
		Content:     append([]byte(nil), content...),
	}, nil
}
