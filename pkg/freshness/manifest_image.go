package freshness

import (
	"regexp"
	"strings"
)

var (
	dockerFrom = regexp.MustCompile(`(?i)^\s*FROM\s+(?:--\S+\s+)*(\S+)`)
	yamlImage  = regexp.MustCompile(`^\s*-?\s*image:\s*["']?([^"'\s#]+)`)
)

// dockerHubHosts are the registry names that mean Docker Hub.
var dockerHubHosts = map[string]bool{"docker.io": true, "index.docker.io": true, "registry-1.docker.io": true}

// ParseImageRef reads `postgres:15`, `docker.io/library/node:20-alpine` or
// `bitnami/redis:7.2`. ok is false for references this package does not look
// up: no tag (latest), a digest, a variable, a non-numeric tag, or any
// registry other than Docker Hub, which may be private.
func ParseImageRef(ref string) (name, version string, ok bool) {
	ref = strings.Trim(strings.TrimSpace(ref), `"'`)
	if ref == "" || strings.ContainsAny(ref, "$@{") {
		return "", "", false
	}
	slash := strings.LastIndex(ref, "/")
	colon := strings.LastIndex(ref, ":")
	if colon <= slash {
		return "", "", false
	}
	repo, tag := ref[:colon], ref[colon+1:]
	parts := strings.Split(repo, "/")
	if len(parts) > 1 && (strings.ContainsAny(parts[0], ".:") || parts[0] == "localhost") {
		if !dockerHubHosts[parts[0]] {
			return "", "", false
		}
		parts = parts[1:]
	}
	if len(parts) == 2 && parts[0] == "library" {
		parts = parts[1:]
	}
	num, _ := leadingDotted(strings.TrimPrefix(tag, "v"))
	if num == "" || len(parts) == 0 || len(parts) > 2 {
		return "", "", false
	}
	return strings.ToLower(strings.Join(parts, "/")), num, true
}

func imageItem(ref, rel string, line int, text string) (Item, bool) {
	name, version, ok := ParseImageRef(ref)
	if !ok {
		return Item{}, false
	}
	return lineItem(EcoImage, name, version, rel, line, text), true
}

func parseDockerfile(rel string, data []byte) []Item {
	var items []Item
	for i, text := range strings.Split(string(data), "\n") {
		if m := dockerFrom.FindStringSubmatch(text); m != nil {
			if it, ok := imageItem(m[1], rel, i+1, text); ok {
				items = append(items, it)
			}
		}
	}
	return items
}

// parseYAMLImages reads `image:` keys of docker-compose files, Kubernetes
// manifests and CI service containers.
func parseYAMLImages(rel string, data []byte) []Item {
	var items []Item
	for i, text := range strings.Split(string(data), "\n") {
		if m := yamlImage.FindStringSubmatch(text); m != nil {
			if it, ok := imageItem(m[1], rel, i+1, text); ok {
				items = append(items, it)
			}
		}
	}
	return items
}
