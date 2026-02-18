// package fsutil provides shared filesystem utility functions.
package fsutil

import (
	"io"
	"os"
)

// CopyFile copies a regular file from src to dst, preserving permissions.
// dst is created or truncated; the parent directory must already exist.
func CopyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	// mirror the source file's permission bits
	sourceInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.Chmod(dst, sourceInfo.Mode())
}
