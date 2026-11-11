package diskgate

import "testing"

func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line  string
		heavy bool
		match string
	}{
		{"go test ./...", true, "go test"},
		{"go build -o /tmp/x ./cmd/auto", true, "go build"},
		{"go install ./cmd/auto", true, "go install"},
		{"go run ./cmd/source-lines --max 300 a.go", true, "go run"},
		{"go -C sub test ./...", true, "go test"},
		{"/usr/local/go/bin/go test ./pkg/...", true, "go test"},
		{"go vet ./...", false, ""},
		{"go version", false, ""},
		{"gofmt -l .", false, ""},
		{"cd internal && go test -run TestX ./cli", true, "go test"},
		{"cd x; go build", true, "go build"},
		{"(cd x && cargo build --release)", true, "cargo build"},
		{"GOFLAGS=-count=1 CGO_ENABLED=0 go test ./...", true, "go test"},
		{"env -i PATH=/bin go test ./...", true, "go test"},
		{"time nice -n 10 go test ./...", true, "go test"},
		{"timeout 60 go test ./...", true, "go test"},
		{"nohup go test ./... > log 2>&1 &", true, "go test"},
		{"bash -c 'cd x && go test ./...'", true, "go test"},
		{`sh -c "npm ci"`, true, "npm ci"},
		{"echo 'go test ./...'", false, ""},
		{`echo "cd x && go test"`, false, ""},
		{"grep -rn 'go build' .", false, ""},
		{"git commit -m 'go test passes'", false, ""},
		{"# go test ./...\nls", false, ""},
		{"ls | grep test", false, ""},
		{"echo $(go build ./...)", true, "go build"},
		{"auto disk lease --need 8 -- go test ./...", false, ""},
		{"auto disk lease -- cargo build", false, ""},
		{"auto disk status", false, ""},
		{"auto update", false, ""},
		{"auto disk lease -- go test ./... && go build ./...", true, "go build"},
		{"cargo test -p x", true, "cargo test"},
		{"cargo +nightly build", true, "cargo build"},
		{"cargo fmt", false, ""},
		{"cargo clippy", false, ""},
		{"npm install", true, "npm install"},
		{"npm i left-pad", true, "npm i"},
		{"npm --prefix web ci", true, "npm ci"},
		{"npm run build", true, "npm run build"},
		{"npm run lint", false, ""},
		{"npm test", true, "npm test"},
		{"pnpm install --frozen-lockfile", true, "pnpm install"},
		{"pnpm -C web build", true, "pnpm build"},
		{"pnpm run build:prod", true, "pnpm run build:prod"},
		{"yarn", true, "yarn install"},
		{"yarn lint", false, ""},
		{"npx tsc --noEmit", false, ""},
		{"docker build -t x .", true, "docker build"},
		{"docker --context remote build .", true, "docker build"},
		{"docker buildx build .", true, "docker buildx build"},
		{"docker compose up -d", true, "docker compose up"},
		{"docker compose -f a.yml up", true, "docker compose up"},
		{"docker compose ps", false, ""},
		{"docker ps", false, ""},
		{"docker-compose up", true, "docker-compose up"},
		{"xcodebuild -scheme App build", true, "xcodebuild"},
		{"xcodebuild -version", false, ""},
		{"git worktree add ../wt -b x", true, "git worktree add"},
		{"git -C repo worktree add ../wt", true, "git worktree add"},
		{"git worktree list", false, ""},
		{"git status", false, ""},
		{"make test", false, ""},
		{"bun install", true, "bun install"},
		{"bun run build", true, "bun run build"},
		{"bun x tsc", false, ""},
		{"pip install -r requirements.txt", true, "pip install"},
		{"pip3 list", false, ""},
		{"python3 -m pip install torch", true, "python3 -m pip install"},
		{"python3 -m pytest", false, ""},
		{"uv sync", true, "uv sync"},
		{"uv pip install x", true, "uv pip install"},
		{"uv run pytest", false, ""},
		{"poetry install", true, "poetry install"},
		{"./gradlew build", true, "gradle build"},
		{"gradle :app:test --info", true, "gradle :app:test"},
		{"./gradlew tasks", false, ""},
		{"mvn -q clean install", true, "mvn install"},
		{"./mvnw package -DskipTests", true, "mvn package"},
		{"mvn dependency:tree", false, ""},
		{"podman build -t x .", true, "podman build"},
		{"podman compose up", true, "podman compose up"},
		{"podman ps", false, ""},
		{"podman-compose up -d", true, "podman-compose up"},
		{"", false, ""},
		{`go test "./pkg/with space/..."`, true, "go test"},
		{"go\\ test", false, ""},
	}
	for _, tc := range cases {
		got := Classify(tc.line)
		if got.Heavy != tc.heavy || got.Match != tc.match {
			t.Errorf("Classify(%q) = %+v, want heavy=%v match=%q", tc.line, got, tc.heavy, tc.match)
		}
	}
}

func TestClassify_ExtraCommands(t *testing.T) {
	t.Parallel()
	extra := []string{"make release", "bazel build", "  ", "terraform"}
	cases := []struct {
		line  string
		match string
	}{
		{"make release -j8", "make release"},
		{"cd x && /usr/bin/make release", "make release"},
		{"make test", ""},
		{"make", ""},
		{"bazel build //...", "bazel build"},
		{"bazel query //...", ""},
		{"terraform plan", "terraform"},
		{"auto disk lease -- make release", ""},
		{"go test ./...", "go test"},
	}
	for _, tc := range cases {
		got := Classify(tc.line, extra...)
		if got.Heavy != (tc.match != "") || got.Match != tc.match {
			t.Errorf("Classify(%q, extra) = %+v, want match %q", tc.line, got, tc.match)
		}
	}
}

func TestLexQuotes(t *testing.T) {
	t.Parallel()
	got := segments(lex(`a 'b c' "d \"e\"" f\ g && h`))
	want := [][]string{{"a", "b c", `d "e"`, "f g"}, {"h"}}
	if len(got) != len(want) {
		t.Fatalf("segments = %q, want %q", got, want)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("segment %d = %q, want %q", i, got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("segment %d word %d = %q, want %q", i, j, got[i][j], want[i][j])
			}
		}
	}
}
