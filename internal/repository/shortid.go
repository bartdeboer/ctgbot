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

func ShortIDFor(id modeluuid.UUID, candidates []modeluuid.UUID, minLength int) (string, error) {
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
	for _, candidate := range candidates {
		if candidate == id {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("id not found: %s", full)
	}

	for length := minLength; length <= len(full); length++ {
		prefix := full[:length]
		if countIDPrefixMatches(candidates, prefix) == 1 {
			return prefix, nil
		}
	}
	return full, nil
}

func ResolveShortID(ref string, candidates []modeluuid.UUID) (modeluuid.UUID, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return modeluuid.Nil, fmt.Errorf("missing id")
	}
	if parsed, err := modeluuid.Parse(ref); err == nil {
		for _, candidate := range candidates {
			if candidate == parsed {
				return parsed, nil
			}
		}
	}

	var matches []modeluuid.UUID
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate.String(), ref) {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		return modeluuid.Nil, fmt.Errorf("id not found: %s", ref)
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
