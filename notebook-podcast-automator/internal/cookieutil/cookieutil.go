package cookieutil

import (
	"sort"
	"strings"
)

type Allowlist struct {
	exact    map[string]struct{}
	prefixes []string
}

// NormalizeCookieHeader removes duplicate cookie names by keeping the last occurrence
// (case-insensitive) and returns a canonical "name=value; name2=value2" string.
//
// This helps when cookies are copied from storage dumps where deleted/older entries
// may still appear in the raw header string.
func NormalizeCookieHeader(cookies string) string {
	cookies = strings.TrimSpace(cookies)
	if cookies == "" {
		return ""
	}

	parts := strings.Split(cookies, ";")
	lastIndex := make(map[string]int, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		lastIndex[strings.ToLower(name)] = i
	}

	kept := make([]string, 0, len(lastIndex))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if lastIndex[strings.ToLower(name)] == i {
			kept = append(kept, part)
		}
	}

	return strings.Join(kept, "; ")
}

// MergeSetCookieHeaders applies Set-Cookie headers to an existing Cookie header.
// It returns the updated Cookie header and the list of cookie names that changed.
//
// Notes:
// - Matching is case-insensitive.
// - Deletions (Max-Age=0 / expired) remove the cookie.
func MergeSetCookieHeaders(cookieHeader string, setCookieHeaders []string) (updated string, changedNames []string) {
	cookieHeader = NormalizeCookieHeader(cookieHeader)
	if cookieHeader == "" || len(setCookieHeaders) == 0 {
		return cookieHeader, nil
	}

	type entry struct {
		name  string
		value string
	}

	parsed := make(map[string]entry)
	for _, part := range strings.Split(cookieHeader, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		parsed[strings.ToLower(name)] = entry{name: name, value: value}
	}

	seenChanged := make(map[string]struct{})
	for _, raw := range setCookieHeaders {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		first, _, _ := strings.Cut(raw, ";")
		name, value, ok := strings.Cut(first, "=")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		lowerName := strings.ToLower(name)
		lowerRaw := strings.ToLower(raw)
		deleteCookie := strings.Contains(lowerRaw, "max-age=0") ||
			strings.Contains(lowerRaw, "expires=thu, 01 jan 1970") ||
			strings.Contains(lowerRaw, "expires=mon, 01 jan 1990")

		if deleteCookie {
			if _, exists := parsed[lowerName]; exists {
				delete(parsed, lowerName)
				if _, already := seenChanged[lowerName]; !already {
					seenChanged[lowerName] = struct{}{}
					changedNames = append(changedNames, name)
				}
			}
			continue
		}

		value = strings.TrimSpace(value)
		if prev, exists := parsed[lowerName]; !exists || prev.value != value {
			if _, already := seenChanged[lowerName]; !already {
				seenChanged[lowerName] = struct{}{}
				changedNames = append(changedNames, name)
			}
		}
		parsed[lowerName] = entry{name: name, value: value}
	}

	if len(parsed) == 0 {
		return "", changedNames
	}

	names := make([]string, 0, len(parsed))
	for _, e := range parsed {
		names = append(names, e.name)
	}
	sort.Strings(names)

	out := make([]string, 0, len(names))
	for _, name := range names {
		e, ok := parsed[strings.ToLower(name)]
		if !ok {
			continue
		}
		out = append(out, e.name+"="+e.value)
	}

	return strings.Join(out, "; "), changedNames
}

func ParseAllowlist(raw string) Allowlist {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Allowlist{}
	}

	al := Allowlist{exact: map[string]struct{}{}}
	for _, item := range splitList(raw) {
		item = strings.TrimSpace(item)
		item = strings.Trim(item, `"'`)
		if item == "" {
			continue
		}

		lower := strings.ToLower(item)
		if strings.HasSuffix(lower, "*") && len(lower) > 1 {
			al.prefixes = append(al.prefixes, strings.TrimSuffix(lower, "*"))
			continue
		}

		al.exact[lower] = struct{}{}
	}
	return al
}

func (a Allowlist) Enabled() bool {
	return len(a.exact) > 0 || len(a.prefixes) > 0
}

func (a Allowlist) Allows(cookieName string) bool {
	if !a.Enabled() {
		return true
	}

	name := strings.ToLower(strings.TrimSpace(cookieName))
	if name == "" {
		return false
	}
	if _, ok := a.exact[name]; ok {
		return true
	}
	for _, prefix := range a.prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func FilterCookieHeader(cookies string, allowlistRaw string) (filtered string, applied bool) {
	al := ParseAllowlist(allowlistRaw)
	if !al.Enabled() {
		return strings.TrimSpace(cookies), false
	}

	cookies = strings.TrimSpace(cookies)
	if cookies == "" {
		return "", true
	}

	parts := strings.Split(cookies, ";")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, _, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		if al.Allows(name) {
			kept = append(kept, part)
		}
	}

	return strings.Join(kept, "; "), true
}

func splitList(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\t', '\n', '\r':
			return true
		default:
			return false
		}
	})
}
