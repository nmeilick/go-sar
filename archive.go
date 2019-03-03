package sar

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/klauspost/pgzip"
	"github.com/pkg/errors"
)

// Type represents the archive type.
type Type int

// Constants to identify various archive types.
const (
	// TypeTar represents the Tar archive type.
	TypeTar Type = 1 + iota
)

// Compressor represents the compressor type.
type Compressor int

// Constants to identify compression type.
const (
	CompressorNone Compressor = iota // CompressNone disables compression.
	CompressorGzip                   // CompressGzip represents the Gzip compressor.
)

// Archive provides access to an archive.
type Archive struct {
	Type       Type       // Type specified the archive type.
	Compressor Compressor // Compressor specifies the compressor to use.
	Writer     io.Writer  // Writer provides sequential writing of an archive.
	ReadLimit  int64      // ReadLimit allows to set a limit to the data read.

	tar       *tar.Writer
	pgzip     *pgzip.Writer
	readbytes int64
	closed    bool
}

// Close closes all ressources used by the archive.
func (a *Archive) Close() error {
	if a.closed {
		return errors.New("already closed")
	}
	a.closed = true

	if a.tar != nil {
		if err := a.tar.Close(); err != nil {
			return errors.Wrap(err, "closing tar")
		}
	}
	if a.pgzip != nil {
		if err := a.pgzip.Close(); err != nil {
			return errors.Wrap(err, "closing gzip")
		}
	}
	return nil
}

// ArchivePath archives the given path.
func (a *Archive) ArchivePath(path string) error {
	base := filepath.Clean(path)

	prepend := ""
	if filepath.Base(os.Args[1]) != "." {
		prepend = filepath.Base(os.Args[1])
	}

	return filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relpath, err := filepath.Rel(base, path)
		if err != nil {
			return errors.Wrap(err, "filepath.Rel")
		}
		switch relpath {
		case ".":
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
}

// AddEntry adds a new file system entry to the archive.
func (a *Archive) AddEntry(path, name string, info os.FileInfo) error {
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
	if err := a.tar.WriteHeader(h); err != nil {
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
		n, err := io.CopyN(a.tar, fd, h.Size)
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
