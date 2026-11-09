//go:build darwin

package main

import (
	"errors"
	"os"
	"os/user"
	"strconv"
)

// operator is the account that called sudo. sudo sets SUDO_UID and SUDO_GID
// itself, so a caller cannot choose them.
type operator struct {
	uid, gid int
}

func callerIdentity() (operator, error) {
	uid, uidErr := canonicalID(os.Getenv("SUDO_UID"))
	gid, gidErr := canonicalID(os.Getenv("SUDO_GID"))
	if uidErr != nil || gidErr != nil || uid == 0 {
		return operator{}, errors.New("the caller must be a non-root account running this through sudo")
	}
	if _, err := user.LookupId(strconv.Itoa(uid)); err != nil {
		return operator{}, errors.New("the calling account does not exist")
	}
	return operator{uid: uid, gid: gid}, nil
}

func canonicalID(value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 || strconv.Itoa(parsed) != value {
		return 0, errors.New("identity is not canonical decimal")
	}
	return parsed, nil
}

func nobodyIdentity() (int, int, error) {
	account, err := user.Lookup("nobody")
	if err != nil {
		return 0, 0, errors.New("the nobody account is unavailable")
	}
	uid, uidErr := canonicalID(account.Uid)
	gid, gidErr := canonicalID(account.Gid)
	if uidErr != nil || gidErr != nil {
		return 0, 0, errors.New("the nobody identity is malformed")
	}
	return uid, gid, nil
}
