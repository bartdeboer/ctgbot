package commandengine

import (
	"bytes"
	"context"
	"fmt"
)

type Engine struct {
	Router              *Router
	Registry            *Registry
	ActiveComponentRefs []string
}

func NewEngine(router *Router, registry *Registry) *Engine {
	return &Engine{Router: router, Registry: registry}
}

func (e *Engine) WithActiveComponentRefs(refs []string) *Engine {
	if e == nil {
		return nil
	}
	e.ActiveComponentRefs = append([]string(nil), refs...)
	return e
}

func (e *Engine) ActiveComponents() []string {
	if e == nil {
		return nil
	}
	return append([]string(nil), e.ActiveComponentRefs...)
}

func (e *Engine) Run(ctx context.Context, base Request, argv []string) (Result, error) {
	if e == nil {
		return Result{}, fmt.Errorf("missing command engine")
	}
	req, err := e.Parse(ctx, base, argv)
	if err != nil {
		return Result{}, err
	}
	return e.Execute(ctx, req)
}

func (e *Engine) Help(ctx context.Context, base Request, scope []string) (Result, error) {
	if e == nil || e.Router == nil {
		return Result{}, fmt.Errorf("missing command router")
	}
	scope = append([]string(nil), scope...)
	var buf bytes.Buffer
	if len(scope) == 0 {
		if err := e.Router.FPrintHelpIndex(ctx, &buf, base.Context.Actor); err != nil {
			return Result{}, err
		}
	} else {
		if err := e.Router.FPrintHelpWithOptions(ctx, &buf, scope, []HelpOption{HelpLitDepth(2)}, base.Context.Actor); err != nil {
			return Result{}, err
		}
	}
	return Result{Text: buf.String()}, nil
}

func (e *Engine) Parse(ctx context.Context, base Request, argv []string) (Request, error) {
	if e == nil || e.Router == nil {
		return Request{}, fmt.Errorf("missing command router")
	}
	return e.Router.Parse(ctx, base, argv)
}

func (e *Engine) Execute(ctx context.Context, req Request) (Result, error) {
	if e == nil || e.Registry == nil {
		return Result{}, fmt.Errorf("missing command registry")
	}
	if e.Router != nil {
		if err := e.Router.Authorize(req); err != nil {
			return Result{}, err
		}
	}
	return e.Registry.Execute(ctx, req)
}

func (e *Engine) Definitions() []Definition {
	if e == nil || e.Router == nil {
		return nil
	}
	return e.Router.Definitions()
}

func (e *Engine) Descriptions() []Description {
	if e == nil || e.Router == nil {
		return nil
	}
	return e.Router.Descriptions()
}
