package sar

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

// Compressor represents the compression type.
type Compressor int

// Constants to identify compression type.
const (
	CompressorNone Compressor = iota // CompressNone disables compression.
	CompressorGzip                   // CompressGzip represents the Gzip compressor.
)

// Archive provides access to an archive.
type Archive struct {
	Type       Type       // Type specifies the archive type.
	Compressor Compressor // Compressor specifies the compressor to use.
	Writer     io.Writer  // Writer provides sequential writing of an archive.
	ReadLimit  int64      // ReadLimit allows to limit the data read.
	WriteLimit int64      // WriteLimit allows to limit the data written, i.e., the archive size.

	setup     bool
	closed    bool
	readbytes int64

	tar   *tar.Writer
	pgzip *pgzip.Writer
}

// LimitData limits the maximum size of the data to be archived.
func (a *Archive) LimitData(limit int64) *Archive {
	a.ReadLimit = limit
	return a
}

// LimitArchive sets the maximum size an archive may grow to.
func (a *Archive) LimitArchive(limit int64) *Archive {
	a.WriteLimit = limit
	return a
}

// Setup initializes resources used by the archive.
func (a *Archive) Setup() error {
	if a.setup {
		return nil
	}

	w := a.Writer
	if a.WriteLimit > 0 {
		w = NewLimitWriter(w, a.WriteLimit)
	}

	switch a.Compressor {
	case CompressorNone:
	case CompressorGzip:
		a.pgzip = pgzip.NewWriter(w)
		_ = a.pgzip.SetConcurrency(512000, runtime.NumCPU()*2)
		w = a.pgzip
	default:
		return errors.New("compressor not supported")
	}

	switch a.Type {
	case TypeTar:
		a.tar = tar.NewWriter(w)
	default:
		return errors.New("archive type not supported")
	}

	a.setup = true
	return nil
}

// Close closes all ressources used by the archive.
func (a *Archive) Close() error {
	if a.closed {
		return errors.New("already closed")
	}
	a.closed = true

	var errlist []string
	if a.tar != nil {
		if err := a.tar.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "closing tar").Error())
		}
	}
	if a.pgzip != nil {
		if err := a.pgzip.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "closing gzip").Error())
		}
	}
	if len(errlist) > 0 {
		return errors.New(strings.Join(errlist, ", "))
	}
	return nil
}

// ArchivePath archives the given path.
func (a *Archive) ArchivePath(path string) error {
	if err := a.Setup(); err != nil {
		return errors.Wrap(err, "setup failed")
	}
	base := filepath.Clean(path)

	prepend := ""
	if filepath.Base(path) != "." {
		prepend = filepath.Base(path)
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
	if err := a.Setup(); err != nil {
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
