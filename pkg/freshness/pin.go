package freshness

import (
	"regexp"
	"strings"
)

// replyPin is a reply line `version-pin: <name> <reason>`; the reason is
// required so a pin always says why.
var replyPin = regexp.MustCompile("(?mi)^[\\s>*-]*`?version-pin:\\s*(\\S+)\\s+\\S")

// ReplyPins reads the names a reply pins deliberately, lower-cased.
func ReplyPins(reply string) map[string]bool {
	pins := map[string]bool{}
	for _, m := range replyPin.FindAllStringSubmatch(reply, -1) {
		pins[strings.ToLower(strings.Trim(m[1], "`'\""))] = true
	}
	return pins
}

// PinnedBy reports whether the item carries a manifest pin comment or is
// named by a reply pin. A reply pin may name the item as `react`,
// `npm:react`, `postgres`, `postgres:13`, or a toolchain as `node`.
func PinnedBy(it Item, pins map[string]bool) bool {
	if it.Pinned {
		return true
	}
	name := strings.ToLower(it.Name)
	spec := strings.ToLower(it.Spec)
	for _, candidate := range []string{name, string(it.Ecosystem) + ":" + name, name + ":" + spec, name + "@" + spec} {
		if pins[candidate] {
			return true
		}
	}
	return false
}
