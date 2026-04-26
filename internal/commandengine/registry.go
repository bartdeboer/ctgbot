package commandengine

import (
	"context"
	"fmt"
	"reflect"
)

type Handler func(ctx context.Context, req Request) (Result, error)

type HandlerFunc[T any] func(ctx context.Context, req Request, cmd T) (Result, error)

type Registry struct {
	handlers map[reflect.Type]Handler
}

func NewRegistry() *Registry {
	return &Registry{handlers: map[reflect.Type]Handler{}}
}

func Register[T any](r *Registry, handler HandlerFunc[T]) error {
	if r == nil {
		return fmt.Errorf("missing command registry")
	}
	if handler == nil {
		return fmt.Errorf("missing command handler")
	}
	commandType := reflect.TypeFor[T]()
	if commandType == nil {
		return fmt.Errorf("missing command type")
	}
	if _, exists := r.handlers[commandType]; exists {
		return fmt.Errorf("duplicate command handler for %s", commandType)
	}
	r.handlers[commandType] = func(ctx context.Context, req Request) (Result, error) {
		command, ok := req.Command.(T)
		if !ok {
			return Result{}, fmt.Errorf("command type mismatch: got %T, want %s", req.Command, commandType)
		}
		return handler(ctx, req, command)
	}
	return nil
}

func (r *Registry) Execute(ctx context.Context, req Request) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("missing command registry")
	}
	if req.Command == nil {
		return Result{}, fmt.Errorf("missing command")
	}
	commandType := reflect.TypeOf(req.Command)
	handler, ok := r.handlers[commandType]
	if !ok {
		return Result{}, fmt.Errorf("unsupported command type %s", commandType)
	}
	return handler(ctx, req)
}
