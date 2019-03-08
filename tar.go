package sar

// NewTar returns a pointer to a new Tar archive structure.
func NewTar() *Archive {
	return &Archive{
		Type:       TypeTar,
		Compressor: CompressorNone,
	}
}

// NewTarGz returns a pointer to a new Tar+Gzip archive structure.
func NewTarGz() *Archive {
	return &Archive{
		Type:       TypeTar,
		Compressor: CompressorGzip,
	}
}
