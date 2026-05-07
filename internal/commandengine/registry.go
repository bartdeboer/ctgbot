package commandengine

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

type Handler func(ctx context.Context, req Request) (Result, error)

type HandlerFunc[T any] func(ctx context.Context, req Request, cmd T) (Result, error)

type Registry struct {
	handlersByPattern map[string]Handler
	handlersByType    map[reflect.Type]Handler
	patternPrefix     string
}

func NewRegistry() *Registry {
	return &Registry{
		handlersByPattern: map[string]Handler{},
		handlersByType:    map[reflect.Type]Handler{},
	}
}

func (r *Registry) WithPatternPrefix(prefix string) *Registry {
	if r == nil {
		return nil
	}
	prefix = NormalizePattern(prefix)
	if prefix == "" {
		return r
	}
	view := &Registry{
		handlersByPattern: r.handlersByPattern,
		handlersByType:    r.handlersByType,
	}
	view.patternPrefix = JoinPattern(r.patternPrefix, prefix)
	return view
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
	if _, exists := r.handlersByType[commandType]; exists {
		return fmt.Errorf("duplicate command handler for %s", commandType)
	}
	r.handlersByType[commandType] = func(ctx context.Context, req Request) (Result, error) {
		command, ok := req.Command.(T)
		if !ok {
			return Result{}, fmt.Errorf("command type mismatch: got %T, want %s", req.Command, commandType)
		}
		return handler(ctx, req, command)
	}
	return nil
}

func RegisterPattern[T any](r *Registry, pattern string, handler HandlerFunc[T]) error {
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
	resolvedPattern, err := r.resolvePattern(pattern)
	if err != nil {
		return err
	}
	if _, exists := r.handlersByPattern[resolvedPattern]; exists {
		return fmt.Errorf("duplicate command handler for %s", resolvedPattern)
	}
	r.handlersByPattern[resolvedPattern] = func(ctx context.Context, req Request) (Result, error) {
		command, ok := req.Command.(T)
		if !ok {
			return Result{}, fmt.Errorf("command type mismatch: got %T, want %s", req.Command, commandType)
		}
		return handler(ctx, req, command)
	}
	return nil
}

func (r *Registry) resolvePattern(pattern string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("missing command registry")
	}
	pattern = NormalizePattern(pattern)
	pattern = JoinPattern(r.patternPrefix, pattern)
	if pattern == "" {
		return "", fmt.Errorf("missing command pattern")
	}
	return pattern, nil
}

func (r *Registry) Execute(ctx context.Context, req Request) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("missing command registry")
	}
	if req.Command == nil {
		return Result{}, fmt.Errorf("missing command")
	}
	if pattern := strings.TrimSpace(req.CanonicalPattern); pattern != "" {
		if handler, ok := r.handlersByPattern[pattern]; ok {
			return handler(ctx, req)
		}
	}
	commandType := reflect.TypeOf(req.Command)
	handler, ok := r.handlersByType[commandType]
	if !ok {
		return Result{}, fmt.Errorf("unsupported command type %s", commandType)
	}
	return handler(ctx, req)
}
