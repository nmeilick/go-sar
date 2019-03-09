package sar

import (
	"archive/tar"
	"io"
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
	Reader     io.Reader  // Reader provides sequential reading of an archive.
	ReadLimit  int64      // ReadLimit allows to limit the data read.
	WriteLimit int64      // WriteLimit allows to limit the data written, i.e., the archive size.

	setup     bool
	closed    bool
	readbytes int64

	tarR   *tar.Reader
	tarW   *tar.Writer
	pgzipR *pgzip.Reader
	pgzipW *pgzip.Writer
}

// WithWriter sets the backing writer.
func (a *Archive) WithWriter(w io.Writer) *Archive {
	a.Writer = w
	return a
}

// WithWriter sets the backing reader.
func (a *Archive) WithReader(r io.Reader) *Archive {
	a.Reader = r
	return a
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

// SetupWriter initializes resources needed for creating an archive.
func (a *Archive) SetupWriter() error {
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
		a.pgzipW = pgzip.NewWriter(w)
		w = a.pgzipW
	default:
		return errors.New("compressor not supported")
	}

	switch a.Type {
	case TypeTar:
		a.tarW = tar.NewWriter(w)
	default:
		return errors.New("archive type not supported")
	}

	a.setup = true
	return nil
}

// SetupReader initializes resources used needed for reading an archive.
func (a *Archive) SetupReader() error {
	if a.setup {
		return nil
	}

	r := a.Reader

	// TODO: Handle limits

	switch a.Compressor {
	case CompressorNone:
	case CompressorGzip:
		if pgzipR, err := pgzip.NewReader(r); err != nil {
			return err
		} else {
			a.pgzipR = pgzipR
		}
		r = a.pgzipR
	default:
		return errors.New("compressor not supported")
	}

	switch a.Type {
	case TypeTar:
		a.tarR = tar.NewReader(r)
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
	if a.tarW != nil {
		if err := a.tarW.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "close tar").Error())
		}
	}
	if a.pgzipW != nil {
		if err := a.pgzipW.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "close gzip").Error())
		}
	}
	if a.pgzipR != nil {
		if err := a.pgzipR.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "close gzip").Error())
		}
	}
	if len(errlist) > 0 {
		return errors.New(strings.Join(errlist, ", "))
	}
	return nil
}
