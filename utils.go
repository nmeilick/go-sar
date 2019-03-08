package sar

import (
	"io"
	"os"
)

func copy(src, dst string) error {
	rstat, err := os.Lstat(src)
	if err != nil {
		return err
	}

	if stat, err := os.Lstat(dst); err == nil && !stat.Mode().IsRegular() {
		if err = os.RemoveAll(dst); err != nil {
			return err
		}
	}

	r, err := os.Open(src)
	if err != nil {
		return err
	}
	defer r.Close()

	w, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, rstat.Mode().Perm())
	if err != nil {
		return err
	}
	defer w.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}

	w.Close()
	os.Chtimes(dst, rstat.ModTime(), rstat.ModTime())
	return nil
}
