package sar

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/klauspost/pgzip"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
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

// SetupWriter initializes resources used needed for creating an archive.
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
		_ = a.pgzipW.SetConcurrency(512000, runtime.NumCPU()*2)
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

	// TODO: Handle limits?

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
			errlist = append(errlist, errors.Wrap(err, "closing tar writer").Error())
		}
	}
	if a.pgzipW != nil {
		if err := a.pgzipW.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "closing gzip").Error())
		}
	}
	if a.pgzipR != nil {
		if err := a.pgzipR.Close(); err != nil {
			errlist = append(errlist, errors.Wrap(err, "closing gzip").Error())
		}
	}
	if len(errlist) > 0 {
		return errors.New(strings.Join(errlist, ", "))
	}
	return nil
}

// ArchivePath archives the given path.
func (a *Archive) ArchivePath(paths ...string) error {
	if err := a.SetupWriter(); err != nil {
		return errors.Wrap(err, "setup failed")
	}

	for _, path := range paths {
		base := filepath.Clean(path)

		prepend := ""
		if filepath.Base(path) != "." {
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

// ExtractOptions specifies how an archive is to be extracted.
type ExtractOptions struct {
	Overwrite         bool
	RestoreOwner      bool
	RestoreTimestamps bool
	FailOnError       bool
}

// NewExtractOptions returns an pointer to an ExtractOptions structure
// initialized with defaults values.
func NewExtractOptions() *ExtractOptions {
	return &ExtractOptions{
		Overwrite:         true,
		RestoreOwner:      false,
		RestoreTimestamps: true,
		FailOnError:       false,
	}
}

// Apply set metadata on a path according to configured rules.
func (o *ExtractOptions) Apply(path string, h *tar.Header) error {
	var problems []string
	if o.RestoreOwner && runtime.GOOS != "windows" {
		if err := os.Chown(path, h.Uid, h.Gid); err != nil {
			problems = append(problems, fmt.Sprintf("setting owner: %s", err))
		}
	}

	if o.RestoreTimestamps {
		if err := os.Chtimes(path, h.AccessTime, h.ModTime); err != nil {
			problems = append(problems, fmt.Sprintf("setting times: %s", err))
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, ", "))
}

// Extract extracts an archive to a given path according to the given rules.
func (a *Archive) Extract(base string, opts *ExtractOptions) error {
	if err := a.SetupReader(); err != nil {
		return errors.Wrap(err, "setup failed")
	}
	defer a.Close()

	seen := map[string]bool{}
	for {
		h, err := a.tarR.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		h.Name = filepath.Clean(h.Name)
		path := filepath.Join(base, h.Name)
		info := h.FileInfo()

		if !opts.Overwrite {
			if _, err := os.Stat(path); err == nil {
				continue
			}
		}

		if dir := filepath.Dir(path); !seen[dir] {
			seen[dir] = true
			os.MkdirAll(dir, 0755)
		}

		switch h.Typeflag {
		case tar.TypeDir:
			err := os.MkdirAll(path, info.Mode().Perm())
			if err != nil {
				fmt.Printf("%s: failed to create directory: %s\n", path, err)
				if opts.FailOnError {
					return errors.Wrap(err, "mkdir failed")
				}
			}
			seen[path] = true
		case tar.TypeReg, tar.TypeRegA:
			fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
			if err != nil {
				fmt.Printf("%s: create failed: %s\n", path, err)
				if opts.FailOnError {
					return errors.Wrap(err, "create failed")
				}
			}
			_, err = io.Copy(fd, a.tarR) // check size?
			if err != nil {
				fmt.Printf("%s: write failed: %s\n", path, err)
				if opts.FailOnError {
					return errors.Wrap(err, "write failed")
				}
			}
		case tar.TypeLink:
			oldname := filepath.Join(base, h.Linkname)
			switch runtime.GOOS {
			case "windows":
				if err := copy(oldname, path); err != nil {
					fmt.Printf("%s: copy failed: %s\n", oldname, err)
					if opts.FailOnError {
						return errors.Wrap(err, "copy failed")
					}
				}
			default:
				os.Remove(path)
				err := os.Link(oldname, path)
				if err != nil {
					fmt.Printf("%s: link failed: %s\n", path, err)
					if opts.FailOnError {
						return errors.Wrap(err, "link failed")
					}
				}
			}
		case tar.TypeSymlink:
			switch runtime.GOOS {
			case "windows":
				oldname := filepath.Join(base, h.Linkname)
				if err := copy(oldname, path); err != nil {
					fmt.Printf("%s: copy failed: %s\n", oldname, err)
					if opts.FailOnError {
						return errors.Wrap(err, "copy failed")
					}
				}
			default:
				os.Remove(path)
				err := os.Symlink(h.Linkname, path)
				if err != nil {
					fmt.Printf("%s: symlink failed: %s\n", path, err)
					if opts.FailOnError {
						return errors.Wrap(err, "symlink failed")
					}
				}
			}
			continue // Do not set attributes
		case tar.TypeChar, tar.TypeBlock:
			dev := unix.Mkdev(uint32(h.Devmajor), uint32(h.Devminor))
			mode := h.Mode
			if h.Typeflag == tar.TypeChar {
				mode |= unix.S_IFCHR
			} else {
				mode |= unix.S_IFBLK
			}

			os.Remove(path)
			err := syscall.Mknod(path, uint32(mode), int(dev))
			if err != nil {
				fmt.Printf("%s: mknod failed: %s\n", path, err)
				if opts.FailOnError {
					return errors.Wrap(err, "mknod failed")
				}
			}
		default:
			fmt.Printf("%s: Unsupported type: %v\n", path, h.Typeflag)
			continue
		}

		if err := opts.Apply(path, h); err != nil {
			if opts.FailOnError {
				return fmt.Errorf("%s: %s", path, err)
			}
		}
	}
	return nil
}
