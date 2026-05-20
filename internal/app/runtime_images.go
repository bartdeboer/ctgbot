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
			if strings.TrimSpace(target.Name) == "" {
				target.Name = registration.Ref()
			}
			targets = append(targets, target)
		}
	}

	return dedupeRuntimeImageTargets(targets)
}

func dedupeRuntimeImageTargets(targets []runtimeimage.Target) ([]runtimeimage.Target, error) {
	flattened := flattenRuntimeImageTargets(targets)
	out := make([]runtimeimage.Target, 0, len(flattened))
	seen := map[string]struct{}{}
	for _, target := range flattened {
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
	target = target.Clean()
	if target.Name == "" {
		target.Name = target.Image
	}
	if target.Dockerfile == "" {
		target.Dockerfile = "Dockerfile"
	}
	return target
}

func runtimeImageTargetSortKey(target runtimeimage.Target) string {
	return strings.Join([]string{target.Name, target.Image, target.Dockerfile}, "\x00")
}

func flattenRuntimeImageTargets(targets []runtimeimage.Target) []runtimeimage.Target {
	out := make([]runtimeimage.Target, 0, len(targets))
	var appendTarget func(runtimeimage.Target)
	appendTarget = func(target runtimeimage.Target) {
		target = target.Clean()
		if target.Uses != nil {
			appendTarget(*target.Uses)
		}
		out = append(out, target)
	}
	for _, target := range targets {
		appendTarget(target)
	}
	return out
}

func orderRuntimeImageTargets(targets []runtimeimage.Target) ([]runtimeimage.Target, error) {
	if len(targets) == 0 {
		return nil, nil
	}
	index := map[string]int{}
	for i, target := range targets {
		for _, key := range runtimeImageTargetKeys(target) {
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
		if targets[i].Uses != nil {
			dep := cleanRuntimeImageTarget(*targets[i].Uses)
			j, ok := firstRuntimeImageTargetIndex(index, dep)
			if !ok {
				return fmt.Errorf("runtime image target %s uses unknown target %s", targets[i].Name, dep.Name)
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

func firstRuntimeImageTargetIndex(index map[string]int, target runtimeimage.Target) (int, bool) {
	for _, key := range runtimeImageTargetKeys(target) {
		i, ok := index[key]
		if ok {
			return i, true
		}
	}
	return 0, false
}

func runtimeImageTargetKeys(target runtimeimage.Target) []string {
	keys := []string{target.Name, target.Image}
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

func (s *service) logf(format string, args ...any) {
	if s != nil && s.Logf != nil {
		s.Logf(format, args...)
	}
}
