package clisetter

import (
	"flag"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/bartdeboer/go-clir"
)

type Setter struct {
	target any
}

func New(target any) *Setter {
	return &Setter{target: target}
}

func (s *Setter) RegisterRoutes(builder *clir.Builder) error {
	if s == nil || s.target == nil {
		return fmt.Errorf("missing clisetter target")
	}

	groups, err := discoverRouteGroups(s.target)
	if err != nil {
		return err
	}
	for _, group := range groups {
		group := group
		builder.Handle(group.path, group.description(), func(req *clir.Request) error {
			_, err := group.handle(req)
			return err
		})
	}
	return nil
}

func (s *Setter) RegisterSubroutes(builder *clir.Builder) error {
	if s == nil || s.target == nil {
		return fmt.Errorf("missing clisetter target")
	}

	groups, err := discoverRouteGroups(s.target)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.path == "" {
			continue
		}
		group := group
		builder.Handle(group.path, group.description(), func(req *clir.Request) error {
			_, err := group.handle(req)
			return err
		})
	}
	return nil
}

func (s *Setter) HandleRoot(args []string) (bool, error) {
	if s == nil || s.target == nil {
		return false, fmt.Errorf("missing clisetter target")
	}

	groups, err := discoverRouteGroups(s.target)
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if group.path != "" {
			continue
		}
		return group.handleArgs(nil, args)
	}
	return false, nil
}

type routeGroup struct {
	path    string
	target  reflect.Value
	methods []methodSpec
}

func (g routeGroup) description() string {
	var names []string
	for _, m := range g.methods {
		names = append(names, m.method.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (g routeGroup) handle(req *clir.Request) (bool, error) {
	return g.handleArgs(req.Params, req.Extra)
}

func (g routeGroup) handleArgs(params clir.Params, extra []string) (bool, error) {
	fs := flag.NewFlagSet(strings.ReplaceAll(g.path, " ", "_"), flag.ContinueOnError)
	bindings := map[string]flagBinding{}

	for _, method := range g.methods {
		for _, field := range method.flagFields {
			if _, exists := bindings[field.flagName]; exists {
				return false, fmt.Errorf("duplicate flag %q on route %q", field.flagName, g.path)
			}
			bindings[field.flagName] = bindFlag(fs, field)
		}
	}

	if err := fs.Parse(extra); err != nil {
		return false, err
	}

	visited := visitedFlags(fs)
	appliedAny := false
	for _, method := range g.methods {
		apply, err := method.shouldApply(visited)
		if err != nil {
			return false, err
		}
		if !apply {
			continue
		}
		arg, err := method.buildInput(params, bindings)
		if err != nil {
			return false, err
		}
		out := method.method.Func.Call([]reflect.Value{g.target, arg})
		if len(out) != 1 {
			return false, fmt.Errorf("method %s must return exactly one value", method.method.Name)
		}
		if !out[0].IsNil() {
			if err, ok := out[0].Interface().(error); ok {
				return false, err
			}
			return false, fmt.Errorf("method %s returned non-error value", method.method.Name)
		}
		appliedAny = true
	}

	return appliedAny, nil
}

type methodSpec struct {
	method      reflect.Method
	inputType   reflect.Type
	routeFields []routeFieldSpec
	flagFields  []flagFieldSpec
}

func (m methodSpec) path() string {
	var parts []string
	for _, field := range m.routeFields {
		if field.segment != "" {
			parts = append(parts, field.segment)
		}
		parts = append(parts, "<"+field.argName+">")
	}
	return strings.Join(parts, " ")
}

func (m methodSpec) shouldApply(visited map[string]bool) (bool, error) {
	if len(m.flagFields) == 0 {
		return true, nil
	}
	for _, field := range m.flagFields {
		if visited[field.flagName] {
			return true, nil
		}
	}
	return false, nil
}

func (m methodSpec) buildInput(params clir.Params, bindings map[string]flagBinding) (reflect.Value, error) {
	input := reflect.New(m.inputType).Elem()

	for _, field := range m.routeFields {
		raw, ok := params[field.argName]
		if !ok {
			return reflect.Value{}, fmt.Errorf("missing route param %q", field.argName)
		}
		value, err := parseScalar(raw, field.typ)
		if err != nil {
			return reflect.Value{}, fmt.Errorf("parse route param %q: %w", field.argName, err)
		}
		input.FieldByIndex(field.index).Set(value)
	}

	for _, field := range m.flagFields {
		binding, ok := bindings[field.flagName]
		if !ok {
			return reflect.Value{}, fmt.Errorf("missing flag binding for %q", field.flagName)
		}
		input.FieldByIndex(field.index).Set(binding.value(field.typ))
	}

	return input, nil
}

type routeFieldSpec struct {
	index   []int
	argName string
	segment string
	typ     reflect.Type
}

type flagFieldSpec struct {
	index    []int
	flagName string
	typ      reflect.Type
}

type flagBinding struct {
	kind reflect.Kind
	str  *string
	i    *int
	i64  *int64
	b    *bool
}

func bindFlag(fs *flag.FlagSet, field flagFieldSpec) flagBinding {
	switch field.typ.Kind() {
	case reflect.String:
		value := ""
		fs.StringVar(&value, field.flagName, "", "")
		return flagBinding{kind: reflect.String, str: &value}
	case reflect.Int:
		value := 0
		fs.IntVar(&value, field.flagName, 0, "")
		return flagBinding{kind: reflect.Int, i: &value}
	case reflect.Int64:
		value := int64(0)
		fs.Int64Var(&value, field.flagName, 0, "")
		return flagBinding{kind: reflect.Int64, i64: &value}
	case reflect.Bool:
		value := false
		fs.BoolVar(&value, field.flagName, false, "")
		return flagBinding{kind: reflect.Bool, b: &value}
	default:
		panic(fmt.Sprintf("unsupported clisetter flag type %s", field.typ))
	}
}

func (b flagBinding) value(typ reflect.Type) reflect.Value {
	switch b.kind {
	case reflect.String:
		return reflect.ValueOf(*b.str).Convert(typ)
	case reflect.Int:
		return reflect.ValueOf(*b.i).Convert(typ)
	case reflect.Int64:
		return reflect.ValueOf(*b.i64).Convert(typ)
	case reflect.Bool:
		return reflect.ValueOf(*b.b).Convert(typ)
	default:
		panic(fmt.Sprintf("unsupported clisetter flag binding kind %s", b.kind))
	}
}

func discoverRouteGroups(target any) ([]routeGroup, error) {
	targetValue := reflect.ValueOf(target)
	targetType := targetValue.Type()
	if targetType.Kind() != reflect.Pointer {
		return nil, fmt.Errorf("clisetter target must be a pointer, got %s", targetType)
	}

	groupMap := map[string]*routeGroup{}
	for i := 0; i < targetType.NumMethod(); i++ {
		method := targetType.Method(i)
		if !strings.HasPrefix(method.Name, "Set") {
			continue
		}
		spec, err := discoverMethodSpec(method)
		if err != nil {
			return nil, err
		}
		path := spec.path()
		group := groupMap[path]
		if group == nil {
			group = &routeGroup{
				path:   path,
				target: targetValue,
			}
			groupMap[path] = group
		}
		group.methods = append(group.methods, spec)
	}

	paths := make([]string, 0, len(groupMap))
	for path := range groupMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	out := make([]routeGroup, 0, len(paths))
	for _, path := range paths {
		out = append(out, *groupMap[path])
	}
	return out, nil
}

func discoverMethodSpec(method reflect.Method) (methodSpec, error) {
	mt := method.Type
	if mt.NumIn() != 2 {
		return methodSpec{}, fmt.Errorf("method %s must take exactly one input struct", method.Name)
	}
	if mt.NumOut() != 1 || !mt.Out(0).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		return methodSpec{}, fmt.Errorf("method %s must return exactly one error", method.Name)
	}

	inputType := mt.In(1)
	if inputType.Kind() != reflect.Struct {
		return methodSpec{}, fmt.Errorf("method %s input must be a struct", method.Name)
	}

	spec := methodSpec{
		method:    method,
		inputType: inputType,
	}
	if err := collectMethodFields(&spec, method.Name, inputType, nil); err != nil {
		return methodSpec{}, err
	}

	return spec, nil
}

func collectMethodFields(spec *methodSpec, methodName string, inputType reflect.Type, prefix []int) error {
	for i := 0; i < inputType.NumField(); i++ {
		field := inputType.Field(i)
		if field.PkgPath != "" {
			continue
		}

		index := append(append([]int{}, prefix...), i)
		argName := strings.TrimSpace(field.Tag.Get("arg"))
		segment := strings.TrimSpace(field.Tag.Get("segment"))
		flagName := strings.TrimSpace(field.Tag.Get("flag"))

		switch {
		case argName != "":
			spec.routeFields = append(spec.routeFields, routeFieldSpec{
				index:   index,
				argName: argName,
				segment: segment,
				typ:     field.Type,
			})
		case flagName != "":
			if !isSupportedScalar(field.Type) {
				return fmt.Errorf("method %s field %s has unsupported flag type %s", methodName, field.Name, field.Type)
			}
			spec.flagFields = append(spec.flagFields, flagFieldSpec{
				index:    index,
				flagName: flagName,
				typ:      field.Type,
			})
		case field.Anonymous && field.Type.Kind() == reflect.Struct:
			if err := collectMethodFields(spec, methodName, field.Type, index); err != nil {
				return err
			}
		default:
			return fmt.Errorf("method %s field %s must declare arg or flag tag", methodName, field.Name)
		}
	}
	return nil
}

func isSupportedScalar(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.String, reflect.Int, reflect.Int64, reflect.Bool:
		return true
	default:
		return false
	}
}

func parseScalar(raw string, typ reflect.Type) (reflect.Value, error) {
	switch typ.Kind() {
	case reflect.String:
		return reflect.ValueOf(raw).Convert(typ), nil
	case reflect.Int:
		var value int
		_, err := fmt.Sscanf(raw, "%d", &value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(value).Convert(typ), nil
	case reflect.Int64:
		var value int64
		_, err := fmt.Sscanf(raw, "%d", &value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(value).Convert(typ), nil
	case reflect.Bool:
		var value bool
		_, err := fmt.Sscanf(raw, "%t", &value)
		if err != nil {
			return reflect.Value{}, err
		}
		return reflect.ValueOf(value).Convert(typ), nil
	default:
		return reflect.Value{}, fmt.Errorf("unsupported scalar type %s", typ)
	}
}

func visitedFlags(fs *flag.FlagSet) map[string]bool {
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}
