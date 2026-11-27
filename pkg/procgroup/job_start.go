package procgroup

// jobStart is the sequence that starts a process inside a Job Object: create
// it suspended, put it in a job, resume it. It is platform-neutral so the
// fallback below can be tested where Job Objects do not exist.
type jobStart[J comparable] struct {
	// start creates the process suspended.
	start func() error
	// join creates a job and assigns the process to it.
	join func() (J, error)
	// resume lets the suspended process run.
	resume func() error
	// abort kills a process that could not be resumed, through its job when
	// it has one (the zero J means none), and returns an *abortedStart so
	// Group.Start reaps it.
	abort func(job J, cause error) error
}

// run starts the process and returns the job it joined, or the zero J when
// it runs without one. A job that cannot be made or joined (a parent job
// that forbids nesting or breakaway, an older Windows) is not an error: the
// process runs without it, and Kill reaches the direct child only, as exec's
// default cancel does. That is a weaker cleanup, not a reason the command
// cannot run, the way Unix carries on without ps. Only a failed start or
// resume is an error: a process that never resumes never runs.
func (s jobStart[J]) run() (J, error) {
	var none J
	if err := s.start(); err != nil {
		return none, err
	}
	job, err := s.join()
	if err != nil {
		job = none
	}
	if err := s.resume(); err != nil {
		return job, s.abort(job, err)
	}
	return job, nil
}
