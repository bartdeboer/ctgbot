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

	return dedupeRuntimeImageTargets(targets), nil
}

func dedupeRuntimeImageTargets(targets []runtimeimage.Target) []runtimeimage.Target {
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
	return out
}

func cleanRuntimeImageTarget(target runtimeimage.Target) runtimeimage.Target {
	target.Name = strings.TrimSpace(target.Name)
	target.Ref = strings.TrimSpace(target.Ref)
	target.Image = strings.TrimSpace(target.Image)
	target.Dockerfile = strings.TrimSpace(target.Dockerfile)
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

func (s *service) logf(format string, args ...any) {
	if s != nil && s.Logf != nil {
		s.Logf(format, args...)
	}
}
