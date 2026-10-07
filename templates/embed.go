// Package templates는 빌트인 템플릿 파일을 Go 바이너리에 임베딩한다.
package templates

import "embed"

// FS는 임베딩된 템플릿 파일시스템이다.
// claude/commands, claude/skills, codex/skills, gemini/skills, shared 하위의 모든 .tmpl 파일이 포함된다.
//
//go:embed claude/commands/*.tmpl claude/skills/*.tmpl claude/*.tmpl claude/rules/*.tmpl claude/workflows/*.tmpl codex/skills/*.tmpl codex/prompts/*.tmpl codex/agents/*.tmpl codex/rules/autopus/*.tmpl codex/*.tmpl gemini/agents/*.tmpl gemini/skills/*/*.tmpl gemini/commands/*.tmpl gemini/commands/auto/*.tmpl gemini/rules/autopus/*.tmpl gemini/settings/*.tmpl shared/*.tmpl
var FS embed.FS
