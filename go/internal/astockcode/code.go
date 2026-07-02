package astockcode

import "strings"

var shenzhenShanghaiPrefixes = []string{
	"000", "001", "002", "003",
	"300", "301",
	"600", "601", "603", "605",
	"688", "689",
}

func Normalize(raw string) string {
	raw = strings.TrimSpace(strings.ToUpper(raw))
	raw = strings.TrimPrefix(raw, "SH.")
	raw = strings.TrimPrefix(raw, "SZ.")
	raw = strings.TrimPrefix(raw, "BJ.")
	raw = strings.TrimSuffix(raw, ".SH")
	raw = strings.TrimSuffix(raw, ".SZ")
	raw = strings.TrimSuffix(raw, ".BJ")
	raw = strings.TrimPrefix(raw, "SH")
	raw = strings.TrimPrefix(raw, "SZ")
	raw = strings.TrimPrefix(raw, "BJ")
	raw = strings.TrimPrefix(raw, "1.")
	raw = strings.TrimPrefix(raw, "0.")
	if len(raw) >= 6 {
		return raw[:6]
	}
	return raw
}

func IsShanghaiShenzhen(raw string) bool {
	code := Normalize(raw)
	if !isSixDigitCode(code) {
		return false
	}
	for _, prefix := range shenzhenShanghaiPrefixes {
		if strings.HasPrefix(code, prefix) {
			return true
		}
	}
	return false
}

func HasResolvedName(code string, name string) bool {
	name = DisplayName(code, name)
	if name == "" {
		return false
	}
	if isSixDigitCode(Normalize(name)) && Normalize(name) == Normalize(code) {
		return false
	}
	return !isAllDigits(name)
}

func DisplayName(code string, name string) string {
	name = strings.TrimSpace(name)
	code = Normalize(code)
	if code != "" && strings.HasPrefix(name, code) {
		name = strings.TrimSpace(strings.TrimPrefix(name, code))
	}
	return name
}

func SQLWhere() string {
	parts := make([]string, 0, len(shenzhenShanghaiPrefixes))
	for _, prefix := range shenzhenShanghaiPrefixes {
		parts = append(parts, "code LIKE '"+prefix+"%'")
	}
	return "(" + strings.Join(parts, " OR ") + ")"
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
