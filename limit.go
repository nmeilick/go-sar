package sar

import "io"

// WriteLimitExceeded is an error object returned when a write would exceed a set limit.
type WriteLimitExceeded struct{}

func (WriteLimitExceeded) Error() string {
	return "written data exceeds the limit set"
}

// ReadLimitExceeded is an error object returned when a read would exceed a set limit.
type ReadLimitExceeded struct{}

func (ReadLimitExceeded) Error() string {
	return "read data exceeds the limit set"
}

// LimitWriter wraps a Writer and returns an error if the written data exceeds a given limit.
type LimitWriter struct {
	Writer io.Writer
	Limit  int64

	written int64
}

// NewLimitWriter returns a pointer to a new LimitWriter wrapping a Writer and limiting
// written data to the given size.
func NewLimitWriter(w io.Writer, limit int64) *LimitWriter {
	return &LimitWriter{
		Writer: w,
		Limit:  limit,
	}
}

// Write writes the data to the underlying writer or returns an error if the size would
// exceed a set limit.
func (w *LimitWriter) Write(p []byte) (int, error) {
	if w.written+int64(len(p)) > w.Limit {
		return 0, WriteLimitExceeded{}
	}
	n, err := w.Writer.Write(p)
	w.written += int64(n)
	return n, err
}
