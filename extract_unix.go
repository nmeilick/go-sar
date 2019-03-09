// +build !windows

package sar

import (
	"archive/tar"
	"os"
	"path/filepath"
	"syscall"

	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
)

func setOwner(path string, uid, gid int) error {
	return os.Chown(path, uid, gid)
}

func (a *Archive) extractHardlink(base string, h *tar.Header, opts *ExtractOptions) error {
	newpath := filepath.Join(base, h.Name)
	oldpath := filepath.Join(base, h.Linkname)

	os.RemoveAll(newpath)
	if err := os.Link(oldpath, newpath); err != nil {
		return errors.Wrap(err, "link")
	}
	return nil
}

func (a *Archive) extractSymlink(base string, h *tar.Header, opts *ExtractOptions) error {
	newpath := filepath.Join(base, h.Name)
	oldpath := filepath.Join(base, h.Linkname)

	os.RemoveAll(newpath)
	if err := os.Symlink(oldpath, newpath); err != nil {
		return errors.Wrap(err, "symlink")
	}
	return nil
}

func (a *Archive) extractDevice(base string, h *tar.Header, opts *ExtractOptions) error {
	path := filepath.Join(base, h.Name)
	dev := unix.Mkdev(uint32(h.Devmajor), uint32(h.Devminor))
	mode := h.Mode
	switch h.Typeflag {
	case tar.TypeChar:
		mode |= unix.S_IFCHR
	case tar.TypeBlock:
		mode |= unix.S_IFBLK
	default:
		return errors.New("unknown device type")
	}

	if err := syscall.Mknod(path, uint32(mode), int(dev)); err != nil {
		return errors.Wrap(err, "mknod")
	}
	return nil
}
