package sar

import (
	"archive/tar"
	"io"
	"runtime"

	"github.com/klauspost/pgzip"
)

// NewTar takes a writer and returns a pointer to a new Tar archive structure.
func NewTar(w io.Writer) *Archive {
	return &Archive{
		Type:       TypeTar,
		Compressor: CompressorNone,
		Writer:     w,

		tar: tar.NewWriter(w),
	}
}

// NewTarGz takes a writer and returns a pointer to a new Tar archive structure
// with Gzip compression enabled.
func NewTarGz(w io.Writer) *Archive {
	gz := pgzip.NewWriter(w)
	gz.SetConcurrency(512000, runtime.NumCPU()*2)
	return &Archive{
		Type:       TypeTar,
		Compressor: CompressorGzip,
		Writer:     w,

		tar:   tar.NewWriter(gz),
		pgzip: gz,
	}
}
