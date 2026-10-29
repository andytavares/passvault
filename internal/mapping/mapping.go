package mapping

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

var (
	envPattern     = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
	commandPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

// Parse decodes mapping.toml text, defaults empty scopes to global, and
// validates the result.
func Parse(data []byte) (Mapping, error) {
	var m Mapping
	meta, err := toml.Decode(string(data), &m)
	if err != nil {
		return Mapping{}, err
	}
	if keys := meta.Undecoded(); len(keys) > 0 {
		return Mapping{}, fmt.Errorf("unknown keys: %s", joinKeys(keys))
	}
	for i := range m.Secrets {
		if m.Secrets[i].Scope == "" {
			m.Secrets[i].Scope = ScopeGlobal
		}
	}
	return m, m.Validate()
}

// Load reads and parses the mapping file at path. Errors wrap the path.
func Load(path string) (Mapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Mapping{}, fmt.Errorf("%s: %w", path, err)
	}
	m, err := Parse(data)
	if err != nil {
		return Mapping{}, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// Validate returns every problem in the mapping, joined.
func (m Mapping) Validate() error {
	var problems []error
	if m.Version == 0 {
		problems = append(problems, errors.New("version is required"))
	} else if m.Version > CurrentVersion {
		problems = append(problems, fmt.Errorf("version %d is newer than this pvault supports, run pvault upgrade", m.Version))
	}
	for i, secret := range m.Secrets {
		problems = append(problems, secret.problems(i)...)
	}
	problems = append(problems, duplicates(m.Secrets)...)
	return errors.Join(problems...)
}

// WriteExample writes Example to path with mode 0600 when path is missing.
// It reports whether it created the file and never overwrites an existing one.
func WriteExample(path string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer file.Close()
	_, err = file.WriteString(Example)
	return err == nil, err
}

// AppendSecret validates s against the file at path, then appends its
// [[secret]] table without touching the existing bytes.
func AppendSecret(path string, s Secret) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	existing, err := Parse(data)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if s.Scope == "" {
		s.Scope = ScopeGlobal
	}
	existing.Secrets = append(existing.Secrets, s)
	if err := existing.Validate(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return appendTable(path, data, renderSecret(s))
}

// RemoveSecret deletes the [[secret]] table for pass from the file at path.
// It edits the text, so comments and the other tables keep their bytes.
func RemoveSecret(path, pass string) error {
	return editTable(path, pass, "")
}

// ReplaceSecret rewrites the [[secret]] table for s.Pass with s. Comments
// inside that table are lost; the rest of the file keeps its bytes.
func ReplaceSecret(path string, s Secret) error {
	if s.Scope == "" {
		s.Scope = ScopeGlobal
	}
	return editTable(path, s.Pass, renderSecret(s))
}

// editTable swaps the [[secret]] table for pass and writes only a valid
// result. Comments after the table's last key belong to the next table and
// stay; an empty replacement also drops the blank lines after the table.
func editTable(path, pass, replacement string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	lines := strings.SplitAfter(string(data), "\n")
	start, end := tableBounds(lines, pass)
	if start < 0 {
		return fmt.Errorf("%s: no [[secret]] with pass %q", path, pass)
	}
	bodyEnd := end
	for bodyEnd > start+1 && isBlankOrComment(lines[bodyEnd-1]) {
		bodyEnd--
	}
	trailing := lines[bodyEnd:]
	for replacement == "" && len(trailing) > 0 && strings.TrimSpace(trailing[0]) == "" {
		trailing = trailing[1:]
	}
	edited := strings.Join(lines[:start], "") + replacement + strings.Join(trailing, "")
	if _, err := Parse([]byte(edited)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return os.WriteFile(path, []byte(edited), 0o600)
}

// tableBounds returns the line range of the [[secret]] table for pass,
// or -1 when no table has that pass.
func tableBounds(lines []string, pass string) (start, end int) {
	start = -1
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "[") {
			continue
		}
		if start >= 0 {
			return start, i
		}
		var table struct {
			Secret []Secret `toml:"secret"`
		}
		_, err := toml.Decode(strings.Join(lines[i:nextHeader(lines, i)], ""), &table)
		if err == nil && len(table.Secret) == 1 && table.Secret[0].Pass == pass {
			start = i
		}
	}
	return start, len(lines)
}

func isBlankOrComment(line string) bool {
	trimmed := strings.TrimSpace(line)
	return trimmed == "" || strings.HasPrefix(trimmed, "#")
}

func nextHeader(lines []string, from int) int {
	for i := from + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "[") {
			return i
		}
	}
	return len(lines)
}

func (s Secret) problems(index int) []error {
	var problems []error
	at := fmt.Sprintf("secret[%d] (%q)", index, s.Pass)
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("%s: %s", at, fmt.Sprintf(format, args...)))
	}
	switch {
	case s.Pass == "":
		add("pass is empty")
	case strings.HasPrefix(s.Pass, "/"):
		add("pass must not have a leading /")
	case hasDotDot(s.Pass):
		add("pass must not contain a .. segment")
	case strings.HasSuffix(s.Pass, ".gpg"):
		add("pass must not have a .gpg suffix")
	}
	problems = append(problems, s.refEnvProblems(at)...)
	return append(problems, s.scopeProblems(at)...)
}

// refEnvProblems checks ref and env; a local entry (no ref) needs env.
func (s Secret) refEnvProblems(at string) []error {
	var problems []error
	switch {
	case s.Ref == "" && s.Env == "":
		problems = append(problems, fmt.Errorf("%s: an entry without ref needs env", at))
	case s.Ref != "" && !strings.HasPrefix(s.Ref, "op://"):
		problems = append(problems, fmt.Errorf("%s: ref must start with op://", at))
	}
	if s.Env != "" && !envPattern.MatchString(s.Env) {
		problems = append(problems, fmt.Errorf("%s: env %q must match [A-Z_][A-Z0-9_]*", at, s.Env))
	}
	return problems
}

func (s Secret) scopeProblems(at string) []error {
	bad := func(format string, args ...any) []error {
		return []error{fmt.Errorf("%s: %s", at, fmt.Sprintf(format, args...))}
	}
	switch s.Scope {
	case "", ScopeGlobal, ScopeNone:
		if len(s.Commands) > 0 {
			return bad("commands require scope = \"command\"")
		}
	case ScopeCommand:
		return s.commandProblems(at)
	default:
		return bad("scope %q must be global, command or none", s.Scope)
	}
	return nil
}

func (s Secret) commandProblems(at string) []error {
	var problems []error
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf("%s: %s", at, fmt.Sprintf(format, args...)))
	}
	if s.Env == "" {
		add("scope = \"command\" requires env")
	}
	if len(s.Commands) == 0 {
		add("scope = \"command\" requires commands")
	}
	for _, command := range s.Commands {
		if !commandPattern.MatchString(command) {
			add("commands entry %q must match [A-Za-z0-9_.-]+", command)
		}
	}
	return problems
}

func duplicates(secrets []Secret) []error {
	var problems []error
	seenPass, seenEnv := map[string]int{}, map[string]int{}
	for i, s := range secrets {
		if first, ok := seenPass[s.Pass]; ok && s.Pass != "" {
			problems = append(problems, fmt.Errorf("secret[%d]: duplicate pass %q (first at secret[%d])", i, s.Pass, first))
		}
		seenPass[s.Pass] = i
		if first, ok := seenEnv[s.Env]; ok && s.Env != "" {
			problems = append(problems, fmt.Errorf("secret[%d]: duplicate env %q (first at secret[%d])", i, s.Env, first))
		}
		seenEnv[s.Env] = i
	}
	return problems
}

func renderSecret(s Secret) string {
	var lines []string
	lines = append(lines, "[[secret]]", fmt.Sprintf("pass = %q", s.Pass))
	if s.Ref != "" {
		lines = append(lines, fmt.Sprintf("ref = %q", s.Ref))
	}
	if s.Env != "" {
		lines = append(lines, fmt.Sprintf("env = %q", s.Env))
	}
	if s.Scope != ScopeGlobal {
		lines = append(lines, fmt.Sprintf("scope = %q", string(s.Scope)))
	}
	if len(s.Commands) > 0 {
		lines = append(lines, fmt.Sprintf("commands = [%s]", quoteJoin(s.Commands)))
	}
	if !s.IsEnabled() {
		lines = append(lines, "enabled = false")
	}
	return strings.Join(lines, "\n") + "\n"
}

func appendTable(path string, data []byte, table string) error {
	separator := ""
	if len(data) > 0 && !strings.HasSuffix(string(data), "\n") {
		separator = "\n"
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(separator + "\n" + table)
	return err
}

func hasDotDot(pass string) bool {
	for _, segment := range strings.Split(pass, "/") {
		if segment == ".." {
			return true
		}
	}
	return false
}

func quoteJoin(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}

func joinKeys(keys []toml.Key) string {
	names := make([]string, len(keys))
	for i, key := range keys {
		names[i] = key.String()
	}
	return strings.Join(names, ", ")
}
