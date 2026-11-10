package securityscan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// plan lists the scanners for the detected ecosystems: gitleaks always,
// then one dependency auditor per ecosystem. No scanner installs anything.
func plan(o Options, ecos []Ecosystem) []job {
	jobs := []job{{name: "gitleaks", bin: "gitleaks", run: runGitleaks}}
	for _, e := range ecos {
		switch e.Name {
		case "go":
			jobs = append(jobs, job{name: "govulncheck", bin: "govulncheck", run: runGovulncheck})
		case "node":
			jobs = append(jobs, nodeJob(e))
		case "rust":
			j := job{name: "cargo audit", bin: "cargo-audit", run: runParsed(parseCargoAudit, "audit", "--json")}
			if e.Manifest != "Cargo.lock" {
				j.skip = "no Cargo.lock to audit; run cargo generate-lockfile"
			}
			jobs = append(jobs, j)
		case "python":
			jobs = append(jobs, pythonJob(e))
		}
	}
	return jobs
}

func nodeJob(e Ecosystem) job {
	switch e.Tool {
	case "npm":
		return job{name: "npm audit", bin: "npm", run: runParsed(parseNodeAudit, "audit", "--json")}
	case "pnpm":
		return job{name: "pnpm audit", bin: "pnpm", run: runParsed(parseNodeAudit, "audit", "--json")}
	case "yarn":
		return job{name: "yarn audit", bin: "yarn", run: runParsed(parseNodeAudit, "audit", "--json")}
	case "yarn-berry":
		return job{name: "yarn npm audit", bin: "yarn", run: runParsed(parseNodeAudit, "npm", "audit", "--json")}
	}
	return job{name: "npm audit", bin: "npm", skip: "no lockfile to audit; install once to create one"}
}

// pythonJob audits requirements files as written: --no-deps with
// --disable-pip makes pip-audit check the listed pins without building an
// environment, so nothing is downloaded or installed.
func pythonJob(e Ecosystem) job {
	j := job{name: "pip-audit", bin: "pip-audit"}
	if !strings.HasPrefix(e.Manifest, "requirements") {
		j.skip = "pip-audit checks requirements files without installing; export one (uv export, poetry export)"
		return j
	}
	args := []string{"-f", "json", "--progress-spinner", "off", "--no-deps", "--disable-pip"}
	for _, r := range strings.Split(e.Manifest, ", ") {
		args = append(args, "-r", r)
	}
	j.run = runParsed(parsePipAudit, args...)
	return j
}

// runGovulncheck uses -format json and falls back to the -json flag of
// releases before v1.1.
func runGovulncheck(ctx context.Context, o Options, path string) ([]Finding, error) {
	findings, err := runParsed(parseGovulncheck, "-format", "json", "./...")(ctx, o, path)
	if err != nil && strings.Contains(err.Error(), "flag provided but not defined") {
		return runParsed(parseGovulncheck, "-json", "./...")(ctx, o, path)
	}
	return findings, err
}

// runGitleaks scans git history (or only staged changes, or a log range)
// when dir is in a git work tree, else the directory. The report goes to a
// temporary file, and gitleaks is told to redact, so the secret value does
// not even reach this process's memory as plain text.
func runGitleaks(ctx context.Context, o Options, path string) ([]Finding, error) {
	report, err := os.CreateTemp("", "autopus-gitleaks-*.json")
	if err != nil {
		return nil, err
	}
	report.Close()
	defer func() { _ = os.Remove(report.Name()) }()
	common := []string{"--no-banner", "--redact", "--report-format", "json", "--report-path", report.Name(), "--exit-code", "0"}
	_, _, probeErr := o.Exec(ctx, o.Dir, path, "dir", "--help")
	modern := probeErr == nil
	var args []string
	switch {
	case !inGitTree(o.Dir) && modern:
		args = append(append([]string{"dir"}, common...), ".")
	case !inGitTree(o.Dir):
		args = append([]string{"detect", "--no-git"}, common...)
	case o.Staged && !modern:
		args = append([]string{"protect", "--staged"}, common...)
	default:
		args = []string{"detect"}
		if modern {
			args = []string{"git"}
		}
		args = append(args, common...)
		if o.Staged && modern {
			args = append(args, "--staged")
		}
		if o.LogOpts != "" {
			args = append(args, "--log-opts", o.LogOpts)
		}
		if modern {
			args = append(args, ".")
		}
	}
	_, stderr, runErr := o.Exec(ctx, o.Dir, path, args...)
	data, _ := os.ReadFile(report.Name())
	if len(strings.TrimSpace(string(data))) == 0 {
		if line := firstLine(stderr); line != "" {
			return nil, errors.New(line)
		}
		if runErr != nil {
			return nil, runErr
		}
		return nil, nil
	}
	return parseGitleaks(data)
}

// inGitTree reports whether dir or an ancestor holds a .git entry.
func inGitTree(dir string) bool {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	for {
		if _, err := os.Stat(filepath.Join(abs, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return false
		}
		abs = parent
	}
}
