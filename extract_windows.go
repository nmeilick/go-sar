// +build windows

package sar

import (
	"archive/tar"
	"path/filepath"

	"github.com/pkg/errors"
)

// setOwner is not supported on Windows.
func setOwner(path string, uid, gid int) error {
	return nil
}

// extractHardlink copies the linked file instead of creating a hard link on Windows.
func (a *Archive) extractHardlink(base string, h *tar.Header, opts *ExtractOptions) error {
	newpath := filepath.Join(base, h.Name)
	oldpath := filepath.Join(base, h.Linkname)

	if err := copy(oldpath, newpath); err != nil {
		return errors.Wrap(err, "copy (hardlink)")
	}
	return nil
}

// extractSymlink copies the linked file instead of creating a symbolic link on Windows.
func (a *Archive) extractSymlink(base string, h *tar.Header, opts *ExtractOptions) error {
	newpath := filepath.Join(base, h.Name)
	oldpath := filepath.Join(base, h.Linkname)

	if err := copy(oldpath, newpath); err != nil {
		return errors.Wrap(err, "copy (symlink)")
	}
	return nil
}

// Extracting device files is not supported on Windows.
func (a *Archive) extractDevice(base string, h *tar.Header, opts *ExtractOptions) error {
	return nil
}
