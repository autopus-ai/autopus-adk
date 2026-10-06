package companionmanifest

import (
	"errors"
	"os"

	"github.com/insajin/autopus-adk/pkg/oslock"
)

func createSignedPairTransactionLock(root *os.Root) (*os.File, error) {
	file, err := root.OpenFile(
		signedPairLockName,
		os.O_RDWR|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if err != nil {
		return nil, errors.New("create signed pair transaction lock")
	}
	lockFile, active, err := validateAndLockSignedPairFile(root, file)
	if err != nil || active {
		if active {
			return nil, errors.New("new signed pair transaction lock is occupied")
		}
		return nil, err
	}
	return lockFile, nil
}

func openSignedPairTransactionLock(root *os.Root) (*os.File, bool, error) {
	file, err := root.OpenFile(signedPairLockName, os.O_RDWR, 0)
	if err != nil {
		return nil, false, errors.New("open signed pair transaction lock")
	}
	return validateAndLockSignedPairFile(root, file)
}

func validateAndLockSignedPairFile(
	root *os.Root,
	file *os.File,
) (*os.File, bool, error) {
	descriptorInfo, descriptorErr := file.Stat()
	pathInfo, pathErr := root.Lstat(signedPairLockName)
	if descriptorErr != nil || pathErr != nil || !descriptorInfo.Mode().IsRegular() ||
		!pathInfo.Mode().IsRegular() || descriptorInfo.Mode().Perm() != 0o600 ||
		pathInfo.Mode().Perm() != 0o600 || !os.SameFile(descriptorInfo, pathInfo) {
		_ = file.Close()
		return nil, false, errors.New("invalid signed pair transaction lock")
	}
	active, err := lockSignedPairFile(file)
	if err != nil || active {
		_ = file.Close()
		return nil, active, err
	}
	return file, false, nil
}

func releaseSignedPairLockFile(file **os.File) error {
	if file == nil || *file == nil {
		return nil
	}
	lockFile := *file
	*file = nil
	return errors.Join(unlockSignedPairFile(lockFile), lockFile.Close())
}

// lockSignedPairFile takes the transaction lock through pkg/oslock and keeps
// the error texts this package reported before the helpers moved there.
func lockSignedPairFile(file *os.File) (bool, error) {
	active, err := oslock.TryLock(file)
	if errors.Is(err, errors.ErrUnsupported) {
		return false, errors.New("signed pair transaction locking is unsupported")
	}
	if err != nil {
		return false, errors.New("lock signed pair transaction")
	}
	return active, nil
}

func unlockSignedPairFile(file *os.File) error {
	err := oslock.Unlock(file)
	if errors.Is(err, errors.ErrUnsupported) {
		return errors.New("signed pair transaction unlocking is unsupported")
	}
	return err
}
