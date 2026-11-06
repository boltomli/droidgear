//go:build !windows

package updater

import "os"

// swapFile replaces target with stagedCopy. On POSIX systems rename(2) over a
// running executable is atomic and the old inode stays alive for the process
// that is still using it, so there is no window in which the executable is
// missing and no backup file to clean up.
func swapFile(stagedCopy, target string) (string, error) {
	if err := os.Rename(stagedCopy, target); err != nil {
		return "", err
	}
	return "", nil
}
