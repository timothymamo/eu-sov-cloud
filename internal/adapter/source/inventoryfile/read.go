package inventoryfile

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// readResult is what the goroutine of read sends back.
type readResult struct {
	data []byte
	err  error
}

// read returns the contents of the file, refusing one larger than MaxFileSize.
//
// The file is opened and read on a goroutine of its own, so that a cancelled
// ctx ends the call even while the open or a read blocks. Cancelling closes the
// file once it is open, which ends a read blocked on a pipe or FIFO. An open
// that blocks (a FIFO with no writer) cannot be interrupted portably, and
// neither can a read Go cannot poll (a regular file on a stalled network file
// system): the goroutine then waits until the open or the read returns, or
// until the process exits, which for the CLI is the end of the run. The call
// returns all the same. The file is read once per call, so a pipe serves a
// single read.
func (s *Source) read(ctx context.Context) ([]byte, error) {
	done := make(chan readResult, 1)

	go func() {
		f, err := os.Open(s.path)
		if err != nil {
			done <- readResult{err: fmt.Errorf("%w: %s", ErrRead, osReason(err))}

			return
		}
		defer f.Close()

		// Closing the file ends a read blocked on it; on the way out, stop
		// keeps a later cancellation from closing it again.
		stop := context.AfterFunc(ctx, func() { _ = f.Close() })
		defer stop()

		data, err := readCapped(f)
		done <- readResult{data: data, err: err}
	}()

	select {
	case r := <-done:
		if r.err != nil && ctx.Err() != nil {
			// The read failed because cancelling closed the file.
			return nil, ctx.Err()
		}

		return r.data, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// readCapped reads r to its end, refusing more than MaxFileSize bytes without
// reading more than one byte past the cap.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRead, osReason(err))
	}

	if len(data) > MaxFileSize {
		return nil, errTooLarge()
	}

	return data, nil
}

// osReason describes why opening or reading the file failed, in the package's
// own words. It never returns the text of an *fs.PathError, which repeats the
// path unquoted: only the OS's reason inside it, quoted.
func osReason(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "no such file"
	case errors.Is(err, fs.ErrPermission):
		return "permission denied"
	}

	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		return text.Quote(pathErr.Err.Error())
	}

	return "unexpected I/O error"
}
