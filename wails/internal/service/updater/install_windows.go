//go:build windows

package updater

import "os"

// swapFile replaces target with stagedCopy on Windows.
//
// A running executable cannot be overwritten or deleted, but it *can* be
// renamed — the image stays mapped under the new name. So the old binary is
// moved aside to `<target>.old`, the new one takes its place, and the backup is
// deleted if the OS lets us (while this process still runs on it, it usually
// does not; CleanupStale removes it on the next start).
//
// If the second rename fails the backup is restored, so the worst case is a
// stale `.old` file, never a missing executable.
func swapFile(stagedCopy, target string) (string, error) {
	backup := target + ".old"
	// A leftover from an earlier update (nothing is running from it now).
	_ = os.Remove(backup)
	if err := os.Rename(target, backup); err != nil {
		return "", err
	}
	if err := os.Rename(stagedCopy, target); err != nil {
		_ = os.Rename(backup, target)
		return "", err
	}
	_ = os.Remove(backup)
	return backup, nil
}
