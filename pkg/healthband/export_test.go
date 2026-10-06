package healthband

// SetRenameForTest replaces the atomic commit step of compaction so a test
// can inject a crash before the rename. Callers must not run in parallel.
func SetRenameForTest(rename func(oldpath, newpath string) error) (restore func()) {
	previous := renameFile
	renameFile = rename
	return func() { renameFile = previous }
}
