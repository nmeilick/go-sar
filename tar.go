package sar

import (
	"io"
)

// NewTar takes a writer and returns a pointer to a new Tar archive structure.
func NewTar(w io.Writer) *Archive {
	return &Archive{
		Type:       TypeTar,
		Compressor: CompressorNone,
		Writer:     w,
	}
}

// NewTarGz takes a writer and returns a pointer to a new Tar archive structure
// with Gzip compression enabled.
func NewTarGz(w io.Writer) *Archive {
	return &Archive{
		Type:       TypeTar,
		Compressor: CompressorGzip,
		Writer:     w,
	}
}
