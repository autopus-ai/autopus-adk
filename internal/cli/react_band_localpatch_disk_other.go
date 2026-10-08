//go:build !darwin && !linux

package cli

import "errors"

// lpNoFollowFlag is unavailable here; the os.Root walk still refuses links.
const lpNoFollowFlag = 0

// lpStatfs cannot read free space here, so every checkout preflight ends
// disk_insufficient (fail-closed).
func lpStatfs(string) (lpDiskSpace, error) {
	return lpDiskSpace{}, errors.New("react band: free space is unknown on this platform")
}
