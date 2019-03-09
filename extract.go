package sar

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// ExtractOptions specifies how an archive is to be extracted.
type ExtractOptions struct {
	Overwrite         bool
	RestoreOwner      bool
	RestoreTimestamps bool
	FailOnError       bool
	ShowErrors        bool
	Errors            []error
}

// NewExtractOptions returns an pointer to an ExtractOptions structure
// initialized with defaults values.
func NewExtractOptions() *ExtractOptions {
	return &ExtractOptions{
		Overwrite:         true,
		RestoreOwner:      false,
		RestoreTimestamps: true,
		FailOnError:       false,
		ShowErrors:        true,
		Errors:            []error{},
	}
}

// Apply set metadata on a path according to configured rules.
func (o *ExtractOptions) Apply(path string, h *tar.Header) error {
	var problems []string
	if o.RestoreOwner {
		if err := setOwner(path, h.Uid, h.Gid); err != nil {
			problems = append(problems, errors.Wrap(err, "setOwner").Error())
		}
	}

	if o.RestoreTimestamps {
		if err := os.Chtimes(path, h.AccessTime, h.ModTime); err != nil {
			problems = append(problems, errors.Wrap(err, "setTimes").Error())
		}
	}

	if len(problems) == 0 {
		return nil
	}
	return errors.New(strings.Join(problems, ", "))
}

// Extract extracts an archive to a given path according to the given rules.
func (a *Archive) Extract(base string, opts *ExtractOptions) error {
	base = cleanPath(base)
	bstat, err := os.Stat(base)
	switch {
	case os.IsNotExist(err):
		return errors.New("destination directory does not exist")
	case err != nil:
		return err
	case !bstat.IsDir():
		return errors.New("destination is not a directory")
	}

	if err := a.SetupReader(); err != nil {
		return errors.Wrap(err, "setup")
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

		h.Name = cleanPath(h.Name)
		h.Linkname = cleanPath(h.Name)
		path := filepath.Join(base, h.Name)

		// Check if the path to extract already exists.
		if _, err := os.Lstat(path); err == nil {
			if !opts.Overwrite {
				continue
			}

			// Remove existing path but ignore errors as problems will manifest themselves
			// during the extract actions and will be handled there.
			_ = os.RemoveAll(path)
		} else {
			// Ensure intermediate directories exist in case the archive is missing those entries.
			if dir := filepath.Dir(path); !seen[dir] {
				seen[dir] = true
				os.MkdirAll(dir, bstat.Mode().Perm())
			}
		}

		err = nil
		switch h.Typeflag {
		case tar.TypeDir:
			err = a.extractDir(base, h, opts)
			seen[path] = true
		case tar.TypeReg, tar.TypeRegA:
			err = a.extractFile(base, h, opts)
		case tar.TypeLink:
			err = a.extractHardlink(base, h, opts)
		case tar.TypeSymlink:
			err = a.extractSymlink(base, h, opts)
			continue // do not set any attributes
		case tar.TypeChar, tar.TypeBlock:
			err = a.extractDevice(base, h, opts)
		default:
			// Ignore unsupported types
			err = fmt.Errorf("unsupported type (%v), skipping", h.Typeflag)
		}

		if err != nil {
			err = errors.Wrap(err, path)
			opts.Errors = append(opts.Errors, err)
			if opts.ShowErrors {
				fmt.Fprintln(os.Stderr, err)
			}
			if opts.FailOnError {
				return err
			}
		} else {
			seen[filepath.Dir(path)] = true
		}

		if err = opts.Apply(path, h); err != nil {
			err = errors.Wrap(err, path)
			if opts.FailOnError {
				return err
			}
		}
	}
	return nil
}

func (a *Archive) extractDir(base string, h *tar.Header, opts *ExtractOptions) error {
	path := filepath.Join(base, h.Name)
	if err := os.MkdirAll(path, h.FileInfo().Mode().Perm()); err != nil {
		return errors.Wrap(err, "mkdir")
	}
	return nil
}

func (a *Archive) extractFile(base string, h *tar.Header, opts *ExtractOptions) error {
	path := filepath.Join(base, h.Name)
	fd, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, h.FileInfo().Mode().Perm())
	if err != nil {
		return errors.Wrap(err, "create")
	}

	if _, err = io.Copy(fd, a.tarR); err != nil {
		return errors.Wrap(err, "write")
	}
	return nil
}
