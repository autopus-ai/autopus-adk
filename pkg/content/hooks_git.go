package content

import "github.com/insajin/autopus-adk/pkg/config"

// generateGitHooks는 .git/hooks/ 스크립트를 생성한다.
// pre-commit: hygiene + arch checks with --staged (only staged files).
// commit-msg: lore format check on the commit message being written.
func generateGitHooks(cfg config.HooksConf) []GitHookScript {
	var hooks []GitHookScript

	if cfg.PreCommitArch {
		hooks = append(hooks, GitHookScript{
			Path:    ".git/hooks/pre-commit",
			Content: buildPreCommitScript(cfg),
		})
	}

	if cfg.PreCommitLore {
		hooks = append(hooks, GitHookScript{
			Path:    ".git/hooks/commit-msg",
			Content: buildCommitMsgScript(),
		})
	}

	return hooks
}

// generateGitOnlyHooks returns checks that have no valid CLI-hook equivalent.
func generateGitOnlyHooks(cfg config.HooksConf) []GitHookScript {
	if !cfg.PreCommitLore {
		return nil
	}
	return []GitHookScript{{
		Path:    ".git/hooks/commit-msg",
		Content: buildCommitMsgScript(),
	}}
}

// autoBinResolver is the shell prelude both git hooks share. The hooks run the
// harness binary found on PATH, which in a self-hosting checkout is an older
// released build than the source being committed — the validation then answers
// for a binary nobody is changing. AUTOPUS_BIN points the hooks at the build
// under test without replacing a signed install.
const autoBinResolver = "AUTO_BIN=\"${AUTOPUS_BIN:-auto}\"\n"

// buildPreCommitScript는 pre-commit 스크립트를 생성한다.
// Uses --staged to only check git-staged files, avoiding submodule/worktree scans.
func buildPreCommitScript(cfg config.HooksConf) string {
	s := "#!/bin/sh\n# Autopus-ADK pre-commit hook (자동 생성)\nset -e\n\n" + autoBinResolver + "\n"

	if cfg.PreCommitArch {
		s += "# 릴리스 hygiene 및 아키텍처 규칙 검사 (staged 파일만)\n\"$AUTO_BIN\" check --hygiene --arch --quiet --staged\n\n"
	}

	s += "exit 0\n"
	return s
}

// buildCommitMsgScript는 commit-msg 스크립트를 생성한다.
// The commit message file path is passed as $1 by git.
func buildCommitMsgScript() string {
	return "#!/bin/sh\n# Autopus-ADK commit-msg hook (자동 생성)\nset -e\n\n" +
		autoBinResolver + "\n" +
		"ROOT=$(git rev-parse --show-toplevel 2>/dev/null || pwd)\ncd \"$ROOT\"\n\n" +
		"# Lore 커밋 메시지 검사\n\"$AUTO_BIN\" check --lore --quiet --message \"$1\"\n" +
		"\"$AUTO_BIN\" lore validate \"$1\"\n\n" +
		"exit 0\n"
}
