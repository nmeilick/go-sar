package sar

import (
	"archive/tar"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	matty "github.com/mattn/go-tty"
	"github.com/pkg/errors"
)

// ExtractOptions specifies how an archive is to be extracted.
type ExtractOptions struct {
	Verbose           bool
	Overwrite         bool
	Interactive       bool
	RestoreOwner      bool
	RestoreTimestamps bool
	FailOnError       bool
	ShowErrors        bool
	Stdout            io.Writer
	Stderr            io.Writer
	Errors            []error
}

// NewExtractOptions returns an pointer to an ExtractOptions structure
// initialized with defaults values.
func NewExtractOptions() *ExtractOptions {
	return &ExtractOptions{
		Verbose:           false,
		Overwrite:         true,
		Interactive:       true,
		RestoreOwner:      false,
		RestoreTimestamps: true,
		FailOnError:       false,
		ShowErrors:        true,
		Stdout:            os.Stdout,
		Stderr:            os.Stderr,
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
	if opts == nil {
		opts = NewExtractOptions()
	}
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

	var tty *matty.TTY
	interactive, overwrite := opts.Interactive, opts.Overwrite
	if interactive {
		switch {
		case isatty.IsTerminal(os.Stdout.Fd()):
		default:
			interactive = false
		}
	}

	hOut, hErr := opts.Stdout, opts.Stderr
	if hOut == nil {
		hOut = ioutil.Discard
	}
	if hErr == nil {
		hOut = ioutil.Discard
	}

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

		// Check if the path to extract to already exists.
		if _, err := os.Lstat(path); err == nil {
			if !overwrite {
				if !interactive {
					fmt.Fprintf(hErr, "Destination already exists, skipping: %s\n", path)
					continue
				}

				if tty == nil {
					var err error
					tty, err = matty.Open()
					if err != nil {
						fmt.Fprintf(hErr, "Failed to open TTY, disable interactive mode: %s\n", err.Error())
						fmt.Fprintf(hErr, "Destination already exists, skipping: %s\n", path)
						interactive = false
						continue
					}
					defer tty.Close()
				}
				fmt.Printf("Overwrite %s (y=yes, n=no, a=all, N=none, q=quit)? ", path)

			input:
				r, err := tty.ReadRune()
				if err != nil {
					fmt.Fprintf(hErr, "Reading from TTY failed, disabling interactive mode: %s\n", err.Error())
					interactive = false
					continue
				}

				skip := true
				switch r {
				case 'y':
					fmt.Print("yes\n")
					skip = false
				case 'n', '\r', '\n':
					fmt.Print("no\n")
				case 'a', 'A':
					fmt.Print("all\n")
					skip = false
					overwrite = true
				case 'N':
					fmt.Print("none\n")
					interactive = false
				case 'q', 'Q':
					fmt.Print("quit\n")
					return ExtractionAbortedError{}
				default:
					goto input
				}
				if skip {
					continue
				}
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
		if opts.Verbose {
			fmt.Fprintf(hOut, "Extracting: %s\n", path)
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

type ExtractionAbortedError struct{}

func (e ExtractionAbortedError) Error() string {
	return "aborted by user"
}
