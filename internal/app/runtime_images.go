package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

func (s *service) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing app storage")
	}

	var targets []runtimeimage.Target
	registrations, err := s.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	for _, registration := range registrations {
		loaded, err := s.resolveLoadedComponent(ctx, registration.ID)
		if err != nil {
			s.logf("runtime image target discovery skipped component=%s err=%v", registration.Ref(), err)
			continue
		}
		provider, ok := loaded.Component.(component.RuntimeImageProvider)
		if !ok {
			continue
		}
		provided, err := provider.RuntimeImageTargets(ctx)
		if err != nil {
			s.logf("runtime image target discovery skipped component=%s err=%v", registration.Ref(), err)
			continue
		}
		for _, target := range provided {
			if strings.TrimSpace(target.Ref) == "" {
				target.Ref = registration.Ref()
			}
			targets = append(targets, target)
		}
	}

	return dedupeRuntimeImageTargets(targets)
}

func dedupeRuntimeImageTargets(targets []runtimeimage.Target) ([]runtimeimage.Target, error) {
	out := make([]runtimeimage.Target, 0, len(targets))
	seen := map[string]struct{}{}
	for _, target := range targets {
		target = cleanRuntimeImageTarget(target)
		if target.Image == "" {
			continue
		}
		key := target.Image + "\x00" + target.Dockerfile
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return runtimeImageTargetSortKey(out[i]) < runtimeImageTargetSortKey(out[j])
	})
	return orderRuntimeImageTargets(out)
}

func cleanRuntimeImageTarget(target runtimeimage.Target) runtimeimage.Target {
	target.Name = strings.TrimSpace(target.Name)
	target.Ref = strings.TrimSpace(target.Ref)
	target.Image = strings.TrimSpace(target.Image)
	target.Dockerfile = strings.TrimSpace(target.Dockerfile)
	target.DependsOn = cleanStringList(target.DependsOn)
	if target.Name == "" {
		target.Name = target.Ref
	}
	if target.Name == "" {
		target.Name = target.Image
	}
	if target.Dockerfile == "" {
		target.Dockerfile = "Dockerfile"
	}
	return target
}

func runtimeImageTargetSortKey(target runtimeimage.Target) string {
	return strings.Join([]string{target.Ref, target.Name, target.Image, target.Dockerfile}, "\x00")
}

func orderRuntimeImageTargets(targets []runtimeimage.Target) ([]runtimeimage.Target, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	index := map[string]int{}
	for i, target := range targets {
		for _, key := range runtimeImageTargetDependencyKeys(target) {
			if _, exists := index[key]; !exists {
				index[key] = i
			}
		}
	}
	visiting := map[int]bool{}
	visited := map[int]bool{}
	var out []runtimeimage.Target
	var visit func(int) error
	visit = func(i int) error {
		if visited[i] {
			return nil
		}
		if visiting[i] {
			return fmt.Errorf("runtime image target dependency cycle at %s", targets[i].Name)
		}
		visiting[i] = true
		for _, dep := range targets[i].DependsOn {
			j, ok := index[dep]
			if !ok {
				return fmt.Errorf("runtime image target %s depends on unknown target %s", targets[i].Name, dep)
			}
			if err := visit(j); err != nil {
				return err
			}
		}
		visiting[i] = false
		visited[i] = true
		out = append(out, targets[i])
		return nil
	}
	for i := range targets {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func runtimeImageTargetDependencyKeys(target runtimeimage.Target) []string {
	keys := []string{target.Name, target.Ref, target.Image}
	out := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func cleanStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *service) logf(format string, args ...any) {
	if s != nil && s.Logf != nil {
		s.Logf(format, args...)
	}
}
