package config

import (
	"fmt"
	"os"
	"sort"
	"strings"
)

// expandEnv expands "${NAME}" and "${NAME:-default}" references in s using
// the process environment. Missing names (unset and without default) are
// returned so callers can fail fast; the reference is kept verbatim in the
// output when missing.
func expandEnv(s string) (string, []string) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	var missing []string
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			end := strings.IndexByte(s[i:], '}')
			if end < 0 { // unterminated reference: keep literally
				b.WriteByte(s[i])
				i++
				continue
			}
			expr := s[i+2 : i+end]
			name, def := expr, ""
			hasDef := false
			if k := strings.Index(expr, ":-"); k >= 0 {
				name, def, hasDef = expr[:k], expr[k+2:], true
			}
			name = strings.TrimSpace(name)
			if name == "" {
				b.WriteString(s[i : i+end+1])
			} else if v, ok := os.LookupEnv(name); ok {
				b.WriteString(v)
			} else if hasDef {
				b.WriteString(def)
			} else {
				missing = append(missing, name)
				b.WriteString(s[i : i+end+1])
			}
			i += end + 1
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), missing
}

// dedupeMissing normalizes and sorts the missing-variable list.
func dedupeMissing(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, n := range in {
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// missingRefError renders a user-facing error for unresolved references.
func missingRefError(kind string, missing []string) error {
	return fmt.Errorf("%s 环境变量未设置: %s（请在部署环境中配置，或使用 ${VAR:-默认值} 语法）",
		kind, strings.Join(dedupeMissing(missing), ", "))
}

// mustRef expands one sensitive field and returns its missing references.
func mustRef(field, s string) []string {
	_, missing := expandEnv(s)
	for i := range missing {
		missing[i] = field + "->" + missing[i]
	}
	return missing
}
