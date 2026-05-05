package commandengine

import (
	"context"
	"fmt"
)

type Engine struct {
	Router   *Router
	Registry *Registry
}

func NewEngine(router *Router, registry *Registry) *Engine {
	return &Engine{Router: router, Registry: registry}
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
