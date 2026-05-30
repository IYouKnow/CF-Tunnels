package apps

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	ScopeResourcesRead = "resources:read"
	ScopeDNSRead       = "dns:read"
	ScopeDNSCreate     = "dns:create"
	ScopeDNSUpdate     = "dns:update"
)

var (
	allowedScopeSet = map[string]struct{}{
		ScopeResourcesRead: {},
		ScopeDNSRead:       {},
		ScopeDNSCreate:     {},
		ScopeDNSUpdate:     {},
	}
	scopeSplitPattern = regexp.MustCompile(`[,\s]+`)
)

func AllowedScopes() []string {
	return []string{
		ScopeResourcesRead,
		ScopeDNSRead,
		ScopeDNSCreate,
		ScopeDNSUpdate,
	}
}

type ScopeList []string

func (s *ScopeList) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*s = ScopeList{}
		return nil
	}

	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*s = ScopeList(list)
		return nil
	}

	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = ScopeList(parseScopeItems(single))
		return nil
	}

	return fmt.Errorf("scopes must be an array of strings or a string")
}

func parseScopeItems(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := scopeSplitPattern.Split(raw, -1)
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		items = append(items, part)
	}
	return items
}

func normalizeScopes(scopes []string) []string {
	normalized := make([]string, 0, len(scopes))
	seen := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		for _, item := range parseScopeItems(scope) {
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			normalized = append(normalized, item)
		}
	}
	return normalized
}

func NormalizeScopes(scopes []string) []string {
	normalized := normalizeScopes(scopes)
	valid := make([]string, 0, len(normalized))
	for _, scope := range normalized {
		if _, ok := allowedScopeSet[scope]; ok {
			valid = append(valid, scope)
		}
	}
	return valid
}

func ValidateScopes(scopes []string) ([]string, error) {
	normalized := normalizeScopes(scopes)
	for _, scope := range normalized {
		if _, ok := allowedScopeSet[scope]; !ok {
			return nil, fmt.Errorf("unknown scope: %s", scope)
		}
	}
	return normalized, nil
}
