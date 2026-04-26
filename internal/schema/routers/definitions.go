package routers

import "github.com/bartdeboer/ctgbot/internal/commandengine"

func definitionsForSource(definitions []commandengine.Definition, source commandengine.Source) []commandengine.Definition {
	out := make([]commandengine.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.AllowsSource(source) {
			out = append(out, definition)
		}
	}
	return out
}
