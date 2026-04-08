package pipeline

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var templatePattern = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

func MatchConditions(context map[string]any, conditions map[string]any) bool {
	for path, expected := range conditions {
		actual, ok := LookupValue(context, path)
		if !ok || !matchesExpected(actual, expected) {
			return false
		}
	}

	return true
}

func LookupValue(context map[string]any, path string) (any, bool) {
	if path == "" {
		return context, true
	}

	current := any(context)
	for _, segment := range strings.Split(path, ".") {
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[segment]
			if !ok {
				return nil, false
			}
			current = next
		default:
			return nil, false
		}
	}

	return current, true
}

func ResolveTemplates(value any, context map[string]any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, entry := range typed {
			result[key] = ResolveTemplates(entry, context)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, entry := range typed {
			result = append(result, ResolveTemplates(entry, context))
		}
		return result
	case string:
		return resolveStringTemplate(typed, context)
	default:
		return value
	}
}

func resolveStringTemplate(value string, context map[string]any) any {
	matches := templatePattern.FindAllStringSubmatch(value, -1)
	if len(matches) == 0 {
		return value
	}

	if len(matches) == 1 && strings.TrimSpace(matches[0][0]) == strings.TrimSpace(value) {
		resolved, ok := LookupValue(context, strings.TrimSpace(matches[0][1]))
		if !ok {
			return value
		}
		return resolved
	}

	resolved := templatePattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := templatePattern.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		entry, ok := LookupValue(context, strings.TrimSpace(parts[1]))
		if !ok {
			return match
		}
		return stringifyValue(entry)
	})

	return resolved
}

func matchesExpected(actual, expected any) bool {
	switch typed := expected.(type) {
	case []any:
		for _, value := range typed {
			if matchesExpected(actual, value) {
				return true
			}
		}
		return false
	case []string:
		for _, value := range typed {
			if matchesExpected(actual, value) {
				return true
			}
		}
		return false
	default:
		return reflect.DeepEqual(normalizeJSONValue(actual), normalizeJSONValue(expected))
	}
}

func stringifyValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		return string(data)
	}
}

func normalizeJSONValue(value any) any {
	switch typed := value.(type) {
	case json.Number:
		return typed.String()
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int8:
		return float64(typed)
	case int16:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint8:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	default:
		return value
	}
}
