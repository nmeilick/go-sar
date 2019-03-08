package main

import (
	"flag"
	"fmt"
	"os"

	sar "github.com/nmeilick/go-sar"
)

// Output an error and exit
func fail(format string, a ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

func main() {
	archive := flag.String("f", "", "Archive file to use")
	force := flag.Bool("force", false, "Overwrite existing files")

	flag.Parse()

	if len(*archive) == 0 {
		fail("Please specify the path of the archive file (-f, use -h for help)!")
	}

	var dst string
	switch len(flag.Args()) {
	case 0:
		fail("Please specify where to extract to!")
	case 1:
		dst = flag.Args()[0]
	default:
		fail("Too many arguments!")
	}

	fd, err := os.Open(*archive)
	if err != nil {
		fail("Failed to open archive: %s", err)
	}

	a := sar.NewTarGz().WithReader(fd)
	opts := sar.NewExtractOptions()
	opts.Overwrite = *force

	if err = a.Extract(dst, opts); err != nil {
		fail("Extraction failed: %s", err)
	}
	a.Close()
	fd.Close()
}
