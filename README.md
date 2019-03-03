# go-sar
Go simple archive handling

This library currently supports creating gzipped tar archives from the filesystem. It uses the excellent
[pgzip library](https://github.com/klauspost/pgzip) for parallel compression.

Examples:
```golang
h, _ := os.OpenFile("test.tar.gz", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
a := sar.NewTarGz(h)
defer a.Close()
if err := a.ArchivePath("/some/file/or/folder"); err != nil {
  log.Fatal(err)
}
```

Limit the size of input and output:
```golang
h, _ := os.OpenFile("test.tar.gz", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
a := sar.NewTarGz(sar.NewLimitWriter(1*1024*1024)) // Limit archive size to 1MiB
a.ReadLimit = 100*1024*1024 // Abort with error if total data is larger than 100MiB
defer a.Close()
if err := a.ArchivePath("/some/file/or/folder"); err != nil {
  log.Fatal(err)
}
```
