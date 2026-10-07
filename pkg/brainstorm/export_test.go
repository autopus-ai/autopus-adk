package brainstorm

// WithBeforeCreate returns opts with a hook that runs after the scan and
// before each exclusive create, so a test can plant a colliding file.
func WithBeforeCreate(opts Options, hook func(path string)) Options {
	opts.beforeCreate = hook
	return opts
}
