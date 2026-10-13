package omp

// ompRootedWriteFailureHook lets a test fail one planned write of a real OMP
// transaction, so rollback is exercised after earlier writes landed. root is
// the workspace the transaction runs in. Production never sets it.
var ompRootedWriteFailureHook func(root, path string, index, total int) error

func (tx *ompRootedTransaction) injectedWriteFailure(path string, index, total int) error {
	if ompRootedWriteFailureHook == nil {
		return nil
	}
	return ompRootedWriteFailureHook(tx.workspace.path, path, index, total)
}
