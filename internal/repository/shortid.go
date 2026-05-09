package repository

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type ShortIDAmbiguousError struct {
	Ref        string
	Candidates []modeluuid.UUID
}

func (e *ShortIDAmbiguousError) Error() string {
	return fmt.Sprintf("short ID %s is ambiguous", strings.TrimSpace(e.Ref))
}

type ShortIDNotFoundError struct {
	Ref string
}

func (e *ShortIDNotFoundError) Error() string {
	return fmt.Sprintf("id not found: %s", strings.TrimSpace(e.Ref))
}

type ShortIDResolver struct {
	ids []modeluuid.UUID
}

func NewShortIDResolver(ids []modeluuid.UUID) *ShortIDResolver {
	return &ShortIDResolver{
		ids: append([]modeluuid.UUID(nil), ids...),
	}
}

func (r *ShortIDResolver) ShortIDFor(id modeluuid.UUID, minLength int) (string, error) {
	if r == nil {
		return "", fmt.Errorf("missing short ID resolver")
	}
	if id.IsNull() {
		return "", fmt.Errorf("missing id")
	}
	full := id.String()
	if minLength <= 0 {
		minLength = 1
	}
	if minLength > len(full) {
		minLength = len(full)
	}

	found := false
	for _, candidate := range r.ids {
		if candidate == id {
			found = true
			break
		}
	}
	if !found {
		return "", &ShortIDNotFoundError{Ref: full}
	}

	for length := minLength; length <= len(full); length++ {
		prefix := full[:length]
		if countIDPrefixMatches(r.ids, prefix) == 1 {
			return prefix, nil
		}
	}
	return full, nil
}

func (r *ShortIDResolver) Resolve(ref string) (modeluuid.UUID, error) {
	if r == nil {
		return modeluuid.Nil, fmt.Errorf("missing short ID resolver")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return modeluuid.Nil, fmt.Errorf("missing id")
	}
	if parsed, err := modeluuid.Parse(ref); err == nil {
		for _, candidate := range r.ids {
			if candidate == parsed {
				return parsed, nil
			}
		}
	}

	var matches []modeluuid.UUID
	for _, candidate := range r.ids {
		if strings.HasPrefix(candidate.String(), ref) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return modeluuid.Nil, &ShortIDNotFoundError{Ref: ref}
	case 1:
		return matches[0], nil
	default:
		sort.Slice(matches, func(i, j int) bool {
			return matches[i].String() < matches[j].String()
		})
		return modeluuid.Nil, &ShortIDAmbiguousError{
			Ref:        ref,
			Candidates: matches,
		}
	}
}

func countIDPrefixMatches(candidates []modeluuid.UUID, prefix string) int {
	count := 0
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate.String(), prefix) {
			count++
		}
	}
	return count
}
