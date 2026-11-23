package audit

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// minKnownBytes keeps short values such as a PIN from replacing common words.
const minKnownBytes = 8

const redacted = "[redacted]"

// Each value pattern skips text that starts with "[", so a value already
// written as [NAME] or [redacted] keeps its name.
var (
	tokenPattern      = regexp.MustCompile(`\b(?:ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|sk-|xoxb-|xoxp-|xoxa-|AKIA|lin_api_)[A-Za-z0-9_-]{16,}`)
	headerPattern     = regexp.MustCompile(`(?i)authorization:\s*[^'"\r\n]+`)
	bearerPattern     = regexp.MustCompile(`(?i)\b(bearer\s+)[^\['"\s][^'"\s]*`)
	flagPattern       = regexp.MustCompile(`(--(?:password|token|secret|api-key)[= ])[^\[\s]\S*`)
	assignmentPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9_]*(?:TOKEN|KEY|SECRET|PASSWORD))=[^\[\s]\S*`)
	secretFlags       = []string{"--password", "--token", "--secret", "--api-key"}
)

// Redact replaces secret text: known values (env name → value) by [NAME],
// then token prefixes, Authorization headers, Bearer values, secret flags
// and assignments.
func Redact(text string, known map[string]string) string {
	text = replaceKnown(text, known)
	text = tokenPattern.ReplaceAllString(text, redacted)
	text = bearerPattern.ReplaceAllString(text, "${1}"+redacted)
	text = headerPattern.ReplaceAllStringFunc(text, redactHeaderValue)
	text = flagPattern.ReplaceAllString(text, "${1}"+redacted)
	return assignmentPattern.ReplaceAllString(text, "${1}="+redacted)
}

// redactHeaderValue redacts the value of one "Authorization: value" match,
// unless the Bearer rule or a known value already replaced it.
func redactHeaderValue(match string) string {
	value := strings.TrimLeft(match[strings.Index(match, ":")+1:], " \t")
	if strings.HasPrefix(value, "[") || strings.HasPrefix(strings.ToLower(value), "bearer [") {
		return match
	}
	return match[:len(match)-len(value)] + redacted
}

// RedactArgv redacts each argument, and the argument after a secret flag.
func RedactArgv(argv []string, known map[string]string) []string {
	out := make([]string, len(argv))
	for index, arg := range argv {
		if index > 0 && slices.Contains(secretFlags, argv[index-1]) {
			out[index] = redacted
			continue
		}
		out[index] = Redact(arg, known)
	}
	return out
}

func replaceKnown(text string, known map[string]string) string {
	names := make([]string, 0, len(known))
	for name, value := range known {
		if len(value) >= minKnownBytes {
			names = append(names, name)
		}
	}
	slices.SortFunc(names, func(left, right string) int {
		return cmp.Compare(len(known[right]), len(known[left]))
	})
	for _, name := range names {
		text = strings.ReplaceAll(text, known[name], "["+name+"]")
	}
	return text
}
