package freshness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// specs flattens items to key=spec for compact assertions.
func specs(items []Item) map[string]string {
	out := map[string]string{}
	for _, it := range items {
		out[it.Key()] = it.Spec
	}
	return out
}

func TestParseFile_EveryEcosystem(t *testing.T) {
	for _, tc := range []struct {
		file, body string
		want       map[string]string
	}{
		{"go.mod", "module x\n\ngo 1.22\n\ntoolchain go1.22.5\n\nrequire github.com/a/b v1.2.0\n\nrequire (\n\tgithub.com/c/d v0.3.0\n\tgithub.com/e/f v1.0.0 // indirect\n)\n",
			map[string]string{"toolchain:go@go": "1.22", "toolchain:go@toolchain": "1.22.5", "go:github.com/a/b": "v1.2.0", "go:github.com/c/d": "v0.3.0"}},
		{"package.json", `{"dependencies":{"react":"^17.0.2","local":"file:../x","alias":"npm:lodash@^4.17.0","gh":"user/repo"},
"devDependencies":{"vite":"~5.0.0"},"engines":{"node":">=18"}}`,
			map[string]string{"npm:react@dependencies": "^17.0.2", "npm:lodash@dependencies": "^4.17.0", "npm:vite@devDependencies": "~5.0.0", "toolchain:node@engines": ">=18"}},
		{"requirements.txt", "Django==4.2.1  # web\n-r base.txt\nrequests>=2.31 ; python_version > '3.8'\nflask\ngit+https://x/y.git\nuvicorn[standard]~=0.30\n",
			map[string]string{"pypi:django": "==4.2.1", "pypi:requests": ">=2.31", "pypi:uvicorn": "~=0.30"}},
		{"pyproject.toml", "[project]\nname = \"x\"\nrequires-python = \">=3.9\"\ndependencies = [\n  \"fastapi>=0.100\",\n  \"uvicorn[standard]==0.23.0\",\n]\n\n[tool.poetry.dependencies]\npython = \"^3.11\"\nPydantic = { version = \"^1.10\", extras = [\"email\"] }\n",
			map[string]string{"toolchain:python@requires-python": ">=3.9", "pypi:fastapi": ">=0.100", "pypi:uvicorn": "==0.23.0", "toolchain:python@poetry": "^3.11", "pypi:pydantic": "^1.10"}},
		{"Cargo.toml", "[package]\nname = \"x\"\nversion = \"0.1.0\"\n\n[dependencies]\nserde = \"1.0\"\ntokio = { version = \"1.28\", features = [\"full\"] }\nlocal = { path = \"../local\" }\nmyrand = { package = \"rand\", version = \"0.7\" }\n\n[dev-dependencies.criterion]\nversion = \"0.4\"\n",
			map[string]string{"crates:serde": "1.0", "crates:tokio": "1.28", "crates:rand": "0.7", "crates:criterion": "0.4"}},
		{"Dockerfile", "FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build\nFROM build AS test\nFROM gcr.io/distroless/static:nonroot\nFROM node:20-alpine\nFROM postgres@sha256:abc\n",
			map[string]string{"image:golang": "1.22", "image:node": "20"}},
		{"docker-compose.yml", "services:\n  db:\n    image: postgres:13\n  cache:\n    image: \"redis:7-alpine\"\n  app:\n    image: ghcr.io/acme/app:1.2\n  mq:\n    image: ${MQ_IMAGE}\n  sql:\n    image: docker.io/library/mysql:8.0\n",
			map[string]string{"image:postgres": "13", "image:redis": "7", "image:mysql": "8.0"}},
		{"k8s/deploy.yaml", "spec:\n  containers:\n    - name: web\n      image: bitnami/nginx:1.25.3\n    - image: busybox\n",
			map[string]string{"image:bitnami/nginx": "1.25.3"}},
		{".nvmrc", "v20.11.0\n", map[string]string{"toolchain:node": "v20.11.0"}},
		{".python-version", "3.11.4\n", map[string]string{"toolchain:python": "3.11.4"}},
		{".tool-versions", "nodejs 20.1.0\ngolang 1.22.0\nruby 3.3.0\n", map[string]string{"toolchain:node": "20.1.0", "toolchain:go": "1.22.0"}},
	} {
		assert.Equal(t, tc.want, specs(ParseFile(tc.file, []byte(tc.body))), tc.file)
	}
	assert.Nil(t, ParseFile(".nvmrc", []byte("lts/*\n")))
	assert.Nil(t, ParseFile("main.go", []byte("package main\n")))
}

func TestParseFile_PinCommentsAndLines(t *testing.T) {
	items := ParseFile("docker-compose.yml", []byte("services:\n  db:\n    image: postgres:13 # version-pin: matches prod RDS\n  cache:\n    image: redis:7\n"))
	require.Len(t, items, 2)
	assert.True(t, items[0].Pinned)
	assert.Equal(t, 3, items[0].Line)
	assert.False(t, items[1].Pinned)
	gomod := ParseFile("go.mod", []byte("module x\n\nrequire github.com/a/b v1.2.0 // version-pin: v2 drops the API we use\n"))
	require.Len(t, gomod, 1)
	assert.True(t, gomod[0].Pinned)
}

func TestScan_WalksManifestsAndReadsNpmrc(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("web/package.json", `{"dependencies":{"@corp/ui":"^1.0.0","react":"^19.0.0"}}`)
	write("web/.npmrc", "@corp:registry=https://npm.corp.example/\n//npm.corp.example/:_authToken=${TOKEN}\n")
	write("node_modules/x/package.json", `{"dependencies":{"old":"^1.0.0"}}`)
	write("deploy/compose.yaml", "services:\n  db:\n    image: postgres:16\n")
	write("testdata/package.json", `{"dependencies":{"fixture":"^1.0.0"}}`)
	items, err := Scan(dir)
	require.NoError(t, err)
	got := specs(items)
	assert.Equal(t, map[string]string{"npm:@corp/ui@dependencies": "^1.0.0", "npm:react@dependencies": "^19.0.0", "image:postgres": "16"}, got)
	for _, it := range items {
		if it.Name == "@corp/ui" {
			assert.Equal(t, "https://npm.corp.example", it.Registry)
		}
		if it.Name == "react" {
			assert.Empty(t, it.Registry, "an unscoped package keeps the public registry")
		}
	}
}
