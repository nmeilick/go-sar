package sar

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// ArchivePath archives the given path.
func (a *Archive) ArchivePath(paths ...string) error {
	if err := a.SetupWriter(); err != nil {
		return errors.Wrap(err, "setup failed")
	}

	for _, path := range paths {
		base := filepath.Clean(path)

		prepend := ""
		switch filepath.Base(path) {
		case "..", ".":
			// Nothing to prepend
		default:
			prepend = filepath.Base(path)
		}

		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			relpath, err := filepath.Rel(base, path)
			if err != nil {
				return errors.Wrap(err, "filepath.Rel")
			}
			switch relpath {
			case ".":
				// TODO: If the given path if a symbolic link, should we dereference it?
				if !info.IsDir() {
					return a.AddEntry(path, filepath.Base(path), info)
				}
				relpath = ""
			}
			if prepend != "" {
				relpath = filepath.Join(prepend, relpath)
			}
			if relpath == "" {
				return nil
			}
			relpath = strings.Replace(relpath, `\`, "/", -1)
			return a.AddEntry(path, relpath, info)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// AddEntry adds a new file system entry to the archive.
func (a *Archive) AddEntry(path, name string, info os.FileInfo) error {
	if err := a.SetupWriter(); err != nil {
		return errors.Wrap(err, "setup failed")
	}

	switch a.Type {
	case TypeTar: // OK
	default:
		return errors.New("archive type not implemented")
	}

	link := ""
	if info.Mode()&os.ModeSymlink != 0 {
		var err error
		if link, err = os.Readlink(path); err != nil {
			return errors.Wrap(err, "readling link")
		}
	}

	h, err := tar.FileInfoHeader(info, link)
	if err != nil {
		return errors.Wrap(err, "FileInfoHeader")
	}

	h.Name = name
	if err := a.tarW.WriteHeader(h); err != nil {
		return errors.Wrap(err, "writing header")
	}

	if info.Mode().IsRegular() {
		fd, err := os.Open(path)
		if err != nil {
			return err
		}
		defer fd.Close()
		if a.ReadLimit > 0 && a.readbytes+h.Size > a.ReadLimit {
			return ReadLimitExceeded{}
		}
		n, err := io.CopyN(a.tarW, fd, h.Size)
		a.readbytes += int64(n)
		if err != nil {
			return errors.Wrap(err, "adding file")
		}
		if n < h.Size {
			return errors.Wrap(err, "short read")
		}
	}
	return nil
}
