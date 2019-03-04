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

Limit the archive size to 10 MiB and the size of the data read to 200 MiB:
```golang
h, _ := os.OpenFile("test.tar.gz", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
a := sar.NewTarGz(h).LimitArchive(10*1024*1024).LimitData(200*1024*1024)
defer a.Close()
if err := a.ArchivePath("/some/file/or/folder"); err != nil {
  log.Fatal(err)
}
```
