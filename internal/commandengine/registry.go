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
	handlersByDefinitionID map[string]Handler
	handlersByType         map[reflect.Type]Handler
	definitionPrefix       string
}

func NewRegistry() *Registry {
	return &Registry{
		handlersByDefinitionID: map[string]Handler{},
		handlersByType:         map[reflect.Type]Handler{},
	}
}

func (r *Registry) WithDefinitionPrefix(prefix string) *Registry {
	if r == nil {
		return nil
	}
	prefix = NormalizePattern(prefix)
	if prefix == "" {
		return r
	}
	view := &Registry{
		handlersByDefinitionID: r.handlersByDefinitionID,
		handlersByType:         r.handlersByType,
	}
	view.definitionPrefix = JoinPattern(r.definitionPrefix, prefix)
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

func RegisterDefinition[T any](r *Registry, definitionID string, handler HandlerFunc[T]) error {
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
	resolvedID, err := r.resolveDefinitionID(definitionID)
	if err != nil {
		return err
	}
	if _, exists := r.handlersByDefinitionID[resolvedID]; exists {
		return fmt.Errorf("duplicate command handler for %s", resolvedID)
	}
	r.handlersByDefinitionID[resolvedID] = func(ctx context.Context, req Request) (Result, error) {
		command, ok := req.Command.(T)
		if !ok {
			return Result{}, fmt.Errorf("command type mismatch: got %T, want %s", req.Command, commandType)
		}
		return handler(ctx, req, command)
	}
	return nil
}

func (r *Registry) resolveDefinitionID(definitionID string) (string, error) {
	if r == nil {
		return "", fmt.Errorf("missing command registry")
	}
	definitionID = NormalizePattern(definitionID)
	definitionID = JoinPattern(r.definitionPrefix, definitionID)
	if definitionID == "" {
		return "", fmt.Errorf("missing command definition id")
	}
	return definitionID, nil
}

func (r *Registry) Execute(ctx context.Context, req Request) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("missing command registry")
	}
	if req.Command == nil {
		return Result{}, fmt.Errorf("missing command")
	}
	if definitionID := strings.TrimSpace(req.DefinitionID); definitionID != "" {
		if handler, ok := r.handlersByDefinitionID[definitionID]; ok {
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
