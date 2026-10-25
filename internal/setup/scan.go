package setup

import (
	"fmt"
	"os"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/block"
)

// ScanZshrcForPass lists the lines of the zshrc at path that run pass
// outside the managed block; those bypass the mapping file.
func ScanZshrcForPass(path string) []string {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var findings []string
	inside := false
	for number, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		switch trimmed {
		case block.Begin:
			inside = true
		case block.End:
			inside = false
		}
		if inside || !runsPass(trimmed) {
			continue
		}
		findings = append(findings, fmt.Sprintf("%s:%d runs pass outside the managed block", path, number+1))
	}
	return findings
}
