package rules

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Everything the DM reads is YAML: the world state block, and every tool result
// that comes back from a call. It used to be a pipe notation of this project's
// own — `Kyle Katarn|id:kyle-katarn|scale:+0` — which was denser but was a
// grammar the model had to be taught, in a prompt that then had to keep teaching
// it. YAML it already knows.
//
// Two styles are in use, and the choice is about shape rather than taste. Block
// style is for a record the DM reads down: an entity, with a line per field.
// Flow style — `{a: 1, b: 2}` — is for a fact it reads across: a roll, a move, a
// group of traits. Flow is what keeps the change affordable, since a tool result
// in block style would be six lines where the pipe notation was one.

// YAMLScalar quotes a value for block style, where it sits alone after its key.
// A plain scalar may hold most things there, so the test is a denylist of what
// actually changes the meaning of the line.
func YAMLScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	needsQuote := s == "" ||
		strings.ContainsAny(s, ":#'\"[]{}|>&*!%@`,") ||
		strings.TrimSpace(s) != s ||
		strings.HasPrefix(s, "-")
	if !needsQuote {
		return s
	}
	return quote(s)
}

// YAMLFlowScalar quotes a value for flow style, where a comma or a colon ends it
// early and takes the rest of the mapping with it.
//
// The test is an allowlist rather than a list of troublesome characters, because
// guessing which punctuation matters is how a comma in "the cargo, no witnesses"
// and a colon in "he said: run" got through and left a block unparseable.
// Interior spaces are fine — a plain scalar may hold them — so most prose still
// renders unquoted; anything else pays two characters and is right.
func YAMLFlowScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	plain := s != "" && strings.TrimSpace(s) == s && strings.IndexFunc(s, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		case r == '-', r == '_', r == '.', r == '+', r == '/', r == ' ':
			return false
		}
		return true
	}) < 0
	if plain {
		return s
	}
	return quote(s)
}

func quote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// YAMLFlowValue renders any value inside a flow mapping — strings, numbers,
// bools, and the lists and maps that arrive as whatever JSON the DM sent.
func YAMLFlowValue(value any) string {
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		return fmt.Sprintf("%t", v)
	case float64, float32, int, int64:
		return fmt.Sprintf("%v", v)
	case string:
		return YAMLFlowScalar(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			parts = append(parts, YAMLFlowValue(item))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]any:
		// Sorted: Go randomises map order, and a rendering that reshuffled
		// between turns would rewrite the cached prompt prefix for nothing.
		parts := make([]string, 0, len(v))
		for _, name := range slices.Sorted(maps.Keys(v)) {
			parts = append(parts, YAMLFlowScalar(name)+": "+YAMLFlowValue(v[name]))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return YAMLFlowScalar(fmt.Sprintf("%v", v))
	}
}

// Flow builds a one-line flow mapping from alternating names and values, in the
// order given. It is what every tool result is made of: a fact the DM reads
// across rather than a record it reads down.
//
// A nil value is dropped rather than rendered as null. Tool results are built by
// listing every field a call could report and letting the absent ones fall away,
// which keeps the caller a single expression instead of a run of ifs.
func Flow(pairs ...any) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		name, ok := pairs[i].(string)
		if !ok || pairs[i+1] == nil {
			continue
		}
		parts = append(parts, YAMLFlowScalar(name)+": "+YAMLFlowValue(pairs[i+1]))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// Field writes one block-style line: an indent, a key, and a scalar.
func Field(b *strings.Builder, indent, name string, value any) {
	switch v := value.(type) {
	case string:
		fmt.Fprintf(b, "%s%s: %s\n", indent, name, YAMLScalar(v))
	default:
		fmt.Fprintf(b, "%s%s: %v\n", indent, name, v)
	}
}
