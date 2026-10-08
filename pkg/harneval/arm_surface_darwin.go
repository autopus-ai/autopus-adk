//go:build darwin

package harneval

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// Fixed inputs of the rebuild, as golden_surface.py and grader.py fix them.
const (
	sandboxExecPath     = "/usr/bin/sandbox-exec"
	graderProfilePath   = "scripts/benchmarks/harness/grader.sb"
	driverVersionSymbol = "github.com/insajin/autopus-adk/pkg/version.version"
	driverBuildTimeout  = 30 * time.Minute
	driverRunTimeout    = 5 * time.Minute
)

// ambientHarnessEntries are the top-level entries workspace.snapshot removes
// from every extracted revision.
var ambientHarnessEntries = []string{".codex", ".claude", ".agents", ".opencode", ".omp", ".gemini", ".autopus",
	"AGENTS.md", "CLAUDE.md", "GEMINI.md", "opencode.json", "autopus.yaml"}

// armBuild is one rebuild in the scratch layout of arm_surface: build/src,
// build/surface_driver, modcache, download, and run.
type armBuild struct {
	ctx                       context.Context
	root, scratch             string
	env                       []string
	goBin, goroot             string
	account, codexHome, owner string
}

func armSurfaceDigest(ctx context.Context, root, commit string, pins Pins, opts ArmSurfaceOptions) (string, error) {
	b := &armBuild{ctx: ctx, env: opts.Env}
	if err := b.locate(root); err != nil {
		return "", err
	}
	scratch, err := os.MkdirTemp("", "harneval-arm-surface-")
	if err != nil {
		return "", err
	}
	defer func() { _ = removeTree(scratch) }()
	if b.scratch, err = filepath.EvalSymlinks(scratch); err != nil {
		return "", err
	}
	src, modcache := filepath.Join(b.scratch, "build", "src"), filepath.Join(b.scratch, "modcache")
	if err := b.extract(commit, src); err != nil {
		return "", fmt.Errorf("revision: %w", err)
	}
	if err := b.installDriver(src); err != nil {
		return "", fmt.Errorf("build: driver package: %w", err)
	}
	if err := b.download(src, modcache, opts.Proxy); err != nil {
		return "", fmt.Errorf("build: module download (pass a proxy when the local module cache lacks a module): %w", err)
	}
	binary := filepath.Join(b.scratch, "build", "surface_driver")
	env := []string{"PATH=" + filepath.Dir(b.goBin), "HOME=" + filepath.Join(b.scratch, "build", "home"),
		"TMPDIR=" + filepath.Join(b.scratch, "build", "tmp"), "GOPATH=" + filepath.Join(b.scratch, "build", "gopath"),
		"GOCACHE=" + filepath.Join(b.scratch, "build", "gocache"), "GOMODCACHE=" + modcache,
		"GOFLAGS=-mod=readonly -buildvcs=false", "GOPROXY=off", "GOSUMDB=off", "GOWORK=off", "GOTOOLCHAIN=local",
		"CGO_ENABLED=0", "PWD=" + src}
	if err := b.sandboxed(filepath.Dir(src), modcache, env, src, driverBuildTimeout, b.goBin, "build", "-trimpath",
		"-ldflags=-X "+driverVersionSymbol+"="+pins.GeneratorVersion, "-o", binary, "./"+SurfaceDriverPackage); err != nil {
		return "", fmt.Errorf("build: %w", err)
	}
	surface, err := b.run(binary, modcache, pins)
	if err != nil {
		return "", fmt.Errorf("generate: %w", err)
	}
	return SurfaceDigest(surface)
}

// locate resolves the checkout, the go binary on PATH and its GOROOT, and
// the account and Codex homes grader.sb keeps closed.
func (b *armBuild) locate(root string) (err error) {
	if b.root, err = filepath.Abs(root); err == nil {
		b.root, err = filepath.EvalSymlinks(b.root)
	}
	if err != nil {
		return fmt.Errorf("checkout: %w", err)
	}
	for _, dir := range filepath.SplitList(envValue(b.env, "PATH")) {
		if info, statErr := os.Stat(filepath.Join(dir, "go")); statErr == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			b.goBin, err = filepath.EvalSymlinks(filepath.Join(dir, "go"))
			break
		}
	}
	if b.goBin == "" || err != nil {
		return fmt.Errorf("build: go toolchain not found on PATH: %v", err)
	}
	out, err := b.output(b.root, []string{"PATH=" + filepath.Dir(b.goBin), "GOTOOLCHAIN=local"}, time.Minute, b.goBin, "env", "GOROOT")
	if err == nil {
		b.goroot, err = filepath.EvalSymlinks(strings.TrimSpace(out))
	}
	if err != nil {
		return fmt.Errorf("build: go env GOROOT: %w", err)
	}
	account, err := user.Current()
	if err != nil {
		return fmt.Errorf("account home: %w", err)
	}
	b.account, b.owner = canonical(account.HomeDir), canonical(os.Getenv("HOME"))
	b.codexHome = canonical(filepath.Join(b.account, ".codex"))
	if codex := os.Getenv("CODEX_HOME"); codex != "" {
		b.codexHome = canonical(codex)
	}
	return nil
}

// extract writes the git archive of commit into dest and removes the
// ambient harness entries, as workspace.snapshot does.
func (b *armBuild) extract(commit, dest string) error {
	archive := filepath.Join(b.scratch, "archive.tar")
	file, err := os.Create(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	cmd := exec.CommandContext(b.ctx, "git", "-C", b.root, "archive", "--format=tar", commit)
	var stderr bytes.Buffer
	cmd.Env, cmd.Stdout, cmd.Stderr = b.env, file, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git archive %s: %v: %s", commit, err, tail(stderr.String()))
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := untar(file, dest); err != nil {
		return err
	}
	for _, name := range ambientHarnessEntries {
		if err := os.RemoveAll(filepath.Join(dest, name)); err != nil {
			return err
		}
	}
	return nil
}

// untar extracts directories, regular files, and links that stay inside dest.
func untar(r io.Reader, dest string) error {
	reader := tar.NewReader(r)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
		name := strings.TrimSuffix(header.Name, "/")
		if header.Typeflag == tar.TypeXGlobalHeader {
			continue
		} else if !isCleanRelPath(name) {
			return fmt.Errorf("archive entry %q escapes the tree", header.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			err = writeEntry(reader, target, header.FileInfo().Mode().Perm()|0o600)
		case tar.TypeSymlink:
			if link := path.Join(path.Dir(name), header.Linkname); path.IsAbs(header.Linkname) || !isCleanRelPath(link) {
				return fmt.Errorf("archive link %s points outside the tree", header.Name)
			}
			err = os.Symlink(header.Linkname, target)
		default:
			return fmt.Errorf("archive entry %s has unsupported type %q", header.Name, header.Typeflag)
		}
		if err != nil {
			return err
		}
	}
}

func writeEntry(r io.Reader, target string, mode fs.FileMode) error {
	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(file, r)
	return errors.Join(err, file.Close())
}

// installDriver makes the checkout's driver the only file of its package in
// src and creates the build's home, tmp, gopath, and gocache.
func (b *armBuild) installDriver(src string) error {
	if err := rejectLinkedComponents(src, SurfaceDriverPackage); err != nil {
		return err
	}
	data, err := readRepoFile(b.root, SurfaceDriverPackage+"/main.go")
	if err != nil {
		return err
	}
	pkg := filepath.Join(src, filepath.FromSlash(SurfaceDriverPackage))
	if err := os.RemoveAll(pkg); err != nil {
		return err
	}
	build := filepath.Dir(src)
	for _, dir := range []string{pkg, build + "/home", build + "/tmp", build + "/gopath", build + "/gocache"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(pkg, "main.go"), data, 0o644)
}

// download fills the session module cache outside any sandbox from a file
// proxy over the local module cache unless proxy names another; the arm's
// go.sum verifies every module and may not change.
func (b *armBuild) download(src, modcache, proxy string) error {
	dl := filepath.Join(b.scratch, "download")
	for _, dir := range []string{modcache, dl + "/home", dl + "/gopath", dl + "/gocache"} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if proxy == "" {
		lookup := append(append([]string{}, b.env...), "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=")
		local, err := b.output(dl, lookup, time.Minute, b.goBin, "env", "GOMODCACHE")
		if err != nil {
			return err
		}
		proxy = (&url.URL{Scheme: "file", Path: filepath.Join(strings.TrimSpace(local), "cache", "download")}).String()
	}
	sums := filepath.Join(src, "go.sum")
	pinned, _ := os.ReadFile(sums)
	env := []string{"PATH=" + filepath.Dir(b.goBin), "HOME=" + dl + "/home", "GOPATH=" + dl + "/gopath",
		"GOCACHE=" + dl + "/gocache", "GOMODCACHE=" + modcache, "GOPROXY=" + proxy, "GOSUMDB=off", "GOFLAGS=-mod=mod",
		"GOWORK=off", "GOTOOLCHAIN=local", "CGO_ENABLED=0"}
	if _, err := b.output(src, env, driverBuildTimeout, b.goBin, "mod", "download"); err != nil {
		return err
	}
	if after, _ := os.ReadFile(sums); !bytes.Equal(after, pinned) {
		return errors.New("go.sum changed during the module download")
	}
	return nil
}

// run writes the default surface under grader.sb into run/surface with an
// empty PATH and HOME; the driver binary and the pinned catalog are copied
// into the run root, the only tree the run may read.
func (b *armBuild) run(binary, modcache string, pins Pins) (string, error) {
	run := filepath.Join(b.scratch, "run")
	for _, dir := range []string{"home", "path", "tmp"} {
		if err := os.MkdirAll(filepath.Join(run, dir), 0o755); err != nil {
			return "", err
		}
	}
	driver, catalog := filepath.Join(run, "surface_driver"), ""
	data, err := os.ReadFile(binary)
	if err == nil {
		err = os.WriteFile(driver, data, 0o755)
	}
	if err == nil && pins.CodexModelCatalog != "" {
		catalog = filepath.Join(run, "codex-models.json")
		if data, err = readRepoFile(b.root, pins.CodexModelCatalog); err == nil {
			err = os.WriteFile(catalog, data, 0o644)
		}
	}
	if err != nil {
		return "", fmt.Errorf("run root: %w", err)
	}
	surface := filepath.Join(run, "surface")
	env := []string{"PATH=" + filepath.Join(run, "path"), "HOME=" + filepath.Join(run, "home"),
		"TMPDIR=" + filepath.Join(run, "tmp"), "PWD=" + filepath.Join(run, "tmp")}
	return surface, b.sandboxed(run, modcache, env, filepath.Join(run, "tmp"), driverRunTimeout, driver,
		"--output", surface, "--project-name", pins.ProjectName, "--generator-version", pins.GeneratorVersion,
		"--codex-model-catalog", catalog, "--codex-cli-version", pins.CodexCLIVersion,
		"--opencode-cli-version", pins.OpencodeCLIVersion)
}
