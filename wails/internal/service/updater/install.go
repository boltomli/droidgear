// Executable replacement shared by both platforms: the staged bytes are first
// copied next to the target (`.new`) so a half-written file can never become
// the executable, then swapped in platform-specifically (see swapFile).
package updater

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// installFile puts a verified staged download in place of target.
func installFile(staged, target string) (string, error) {
	info, err := os.Stat(staged)
	if err != nil {
		return "", fmt.Errorf("staged update is missing: %w", err)
	}
	mode := info.Mode().Perm()
	if mode&0o111 == 0 {
		mode |= 0o755
	}
	stagedCopy := target + ".new"
	if err := copyFile(staged, stagedCopy, mode); err != nil {
		_ = os.Remove(stagedCopy)
		return "", permissionHint(target, err)
	}
	backup, err := swapFile(stagedCopy, target)
	if err != nil {
		_ = os.Remove(stagedCopy)
		return "", permissionHint(target, err)
	}
	return backup, nil
}

// permissionHint turns the opaque EACCES/EPERM a read-only install directory
// produces into an actionable message.
func permissionHint(target string, err error) error {
	if errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf(
			"cannot replace %s — move the binary to a writable folder or run with elevated rights: %w",
			target, err)
	}
	return err
}

// copyFile copies src to dst with the given permissions and flushes it to
// disk, so the swap never installs a partially written executable.
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src) // #nosec G304 -- staged download inside the cache dir
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode) // #nosec G304 -- target sibling file
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
