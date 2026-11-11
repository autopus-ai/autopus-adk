package diskspace

import (
	"path/filepath"
)

// Consumer names of the auto-detected consumers. Each is measured only when
// its directory exists; an absent one is left out, never reported as zero.
const (
	ConsumerGoModCache       = "go_mod_cache"
	ConsumerNpmCache         = "npm_cache"
	ConsumerPnpmStore        = "pnpm_store"
	ConsumerYarnCache        = "yarn_cache"
	ConsumerBunCache         = "bun_cache"
	ConsumerCargoRegistry    = "cargo_registry"
	ConsumerPipCache         = "pip_cache"
	ConsumerUvCache          = "uv_cache"
	ConsumerGradleCache      = "gradle_cache"
	ConsumerMavenRepo        = "maven_repo"
	ConsumerXcodeDerivedData = "xcode_derived_data"
	ConsumerXcodeSimulators  = "xcode_simulators"
	ConsumerDockerDesktop    = "docker_desktop"
	ConsumerColima           = "colima"
	ConsumerLima             = "lima"
	ConsumerPodman           = "podman"
	ConsumerDockerRoot       = "docker_root"
	// ConsumerRootPrefix names a disk.consumers.extra_roots entry: "root:<path>".
	ConsumerRootPrefix = "root:"
)

// knownRoots lists the heavy consumers common on developer machines for
// goos, under home, with the environment overrides each tool honours. Paths
// that do not exist are dropped by the measurement, so a missing tool costs
// one Lstat.
func knownRoots(in ExtraConsumerInput) []extraRoot {
	env := in.Getenv
	if env == nil {
		env = func(string) string { return "" }
	}
	home := in.Home
	if home == "" {
		return nil
	}
	join := func(parts ...string) string { return filepath.Join(append([]string{home}, parts...)...) }
	or := func(key, fallback string) string {
		if v := env(key); filepath.IsAbs(v) {
			return v
		}
		return fallback
	}
	cache := or("XDG_CACHE_HOME", join(".cache"))
	data := or("XDG_DATA_HOME", join(".local", "share"))
	roots := []extraRoot{
		{name: ConsumerNpmCache, path: or("npm_config_cache", join(".npm"))},
		{name: ConsumerBunCache, path: join(".bun", "install", "cache")},
		{name: ConsumerCargoRegistry, path: filepath.Join(or("CARGO_HOME", join(".cargo")), "registry")},
		{name: ConsumerUvCache, path: or("UV_CACHE_DIR", filepath.Join(cache, "uv"))},
		{name: ConsumerGradleCache, path: filepath.Join(or("GRADLE_USER_HOME", join(".gradle")), "caches")},
		{name: ConsumerMavenRepo, path: join(".m2", "repository")},
		{name: ConsumerColima, path: join(".colima")},
		{name: ConsumerLima, path: join(".lima")},
		{name: ConsumerPodman, path: filepath.Join(data, "containers")},
	}
	if in.GoModCache != "" {
		roots = append([]extraRoot{{name: ConsumerGoModCache, path: in.GoModCache}}, roots...)
	}
	switch in.GOOS {
	case "darwin":
		roots = append(roots,
			extraRoot{name: ConsumerPnpmStore, path: join("Library", "pnpm", "store")},
			extraRoot{name: ConsumerYarnCache, path: join("Library", "Caches", "Yarn")},
			extraRoot{name: ConsumerPipCache, path: join("Library", "Caches", "pip")},
			extraRoot{name: ConsumerXcodeDerivedData, path: join("Library", "Developer", "Xcode", "DerivedData")},
			extraRoot{name: ConsumerXcodeSimulators, path: join("Library", "Developer", "CoreSimulator", "Devices")},
			extraRoot{name: ConsumerDockerDesktop, path: join("Library", "Containers", "com.docker.docker", "Data")})
	case "linux":
		roots = append(roots,
			extraRoot{name: ConsumerPnpmStore, path: filepath.Join(data, "pnpm", "store")},
			extraRoot{name: ConsumerYarnCache, path: filepath.Join(cache, "yarn")},
			extraRoot{name: ConsumerPipCache, path: filepath.Join(cache, "pip")},
			extraRoot{name: ConsumerDockerDesktop, path: join(".docker", "desktop", "vms")},
			extraRoot{name: ConsumerDockerRoot, path: "/var/lib/docker"})
	}
	for i := range roots {
		roots[i].optional = true
	}
	return roots
}
