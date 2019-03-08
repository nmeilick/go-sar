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
	amax := flag.Int("limit-archive", 0, "Abort with an error if the archive size exceeds this limit")
	dmax := flag.Int("limit-data", 0, "Abort with an error if the data to archive exceeds this limit")

	flag.Parse()

	if len(*archive) == 0 {
		fail("Please specify the path of the archive file (-f, use -h for help)!")
	}

	if len(flag.Args()) == 0 {
		fail("Please specify one or more paths to archive!")
	}

	fd, err := os.OpenFile(*archive, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fail("Failed to create archive: %s", err)
	}

	a := sar.NewTarGz().WithWriter(fd)
	if *amax > 0 {
		a.LimitArchive(int64(*amax))
	}
	if *dmax > 0 {
		a.LimitData(int64(*dmax))
	}

	if err = a.ArchivePath(flag.Args()...); err != nil {
		fail("Archiving failed: %s", err)
	}
	a.Close()
	fd.Close()
}
