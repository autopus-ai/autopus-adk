package content

import (
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

func isOMPSafeIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			(index > 0 && (char == '-' || char == '_')) {
			continue
		}
		return false
	}
	return true
}

// OMPYAMLScalar keeps skill and command metadata inside a single YAML value.
func OMPYAMLScalar(value string) string {
	encoded, err := yaml.Marshal(value)
	if err != nil {
		return strconv.Quote(value)
	}
	return strings.TrimRight(string(encoded), "\n")
}
