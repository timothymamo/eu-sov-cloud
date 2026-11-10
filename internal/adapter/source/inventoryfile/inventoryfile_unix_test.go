//go:build unix

package inventoryfile_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// newFIFO creates a FIFO in a fresh directory and returns its path.
func newFIFO(t *testing.T) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "fifo.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("creating the FIFO %s: %v", text.Quote(path), err)
	}

	return path
}

// releaseStuckOpen opens the FIFO at path for writing and closes it, so that a
// goroutine of Inventory blocked in open, for a writer that never came, is
// released instead of being left behind for the run of the test binary.
//
// The open is tried without blocking first, which is enough where a reader that
// is only waiting in open already counts as a reader. Where it does not, the
// open blocks until the waiting reader pairs with it, so it runs on a goroutine
// of its own, and a bounded wait keeps a FIFO without a waiting reader from
// hanging the test.
func releaseStuckOpen(t *testing.T, path string) {
	t.Helper()

	t.Cleanup(func() {
		if w, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = w.Close()

			return
		}

		released := make(chan struct{})

		go func() {
			defer close(released)

			w, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err != nil {
				return
			}

			_ = w.Close()
		}()

		select {
		case <-released:
		case <-time.After(time.Second):
		}
	})
}

// TestInventoryReturnsWhenTheFIFOHasNoWriter pins that a read that would block
// still ends when the context is cancelled: opening a FIFO waits for a writer
// that never comes, so only a context-aware open returns at all.
//
// The test is Unix-only, since it needs syscall.Mkfifo, and not parallel, since
// it measures how long the call takes.
func TestInventoryReturnsWhenTheFIFOHasNoWriter(t *testing.T) {
	path := newFIFO(t)

	// No writer is opened by the test: the goroutine of Inventory stays in open,
	// which is what the cancellation must not wait for.
	releaseStuckOpen(t, path)

	// The context is cancelled, not timed out: a deadline would report
	// context.DeadlineExceeded and say nothing about cancellation.
	ctx, cancel := context.WithCancel(t.Context())

	timer := time.AfterFunc(50*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()

	// Buffered, so the goroutine never blocks on the send even when the test has
	// already given up on it.
	done := make(chan error, 1)

	go func() {
		_, err := inventoryfile.New(path).Inventory(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Inventory() on a FIFO with no writer and a cancelled context returned error %v, want one wrapping %v",
				err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatalf("Inventory() on a FIFO with no writer did not return within 1s of its context being cancelled")
	}
}

// TestInventoryCancelClosesTheFileOfABlockedRead pins that cancelling a read
// that is already blocked on the open file closes the file: Inventory returns
// the cancellation, and the read end is gone, which a write to the FIFO proves
// by failing with EPIPE.
//
// A reader that is left open keeps the FIFO alive and, for a FIFO that is never
// written, blocks for the rest of the run. Only the close is what ends it.
//
// The test is Unix-only, since it needs syscall.Mkfifo, and not parallel, since
// it measures how long the call takes.
func TestInventoryCancelClosesTheFileOfABlockedRead(t *testing.T) {
	path := newFIFO(t)

	// The writer opens first and waits for Inventory to open the read end; once
	// both are open nothing is written, so the read blocks.
	opened := make(chan *os.File, 1)
	openErr := make(chan error, 1)

	go func() {
		w, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			openErr <- err

			return
		}

		opened <- w
	}()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Buffered, so the goroutine never blocks on the send even when the test has
	// already given up on it.
	done := make(chan error, 1)

	go func() {
		_, err := inventoryfile.New(path).Inventory(ctx)
		done <- err
	}()

	var writer *os.File

	select {
	case writer = <-opened:
	case err := <-openErr:
		t.Fatalf("opening the FIFO %s for writing: %v", text.Quote(path), err)
	case <-time.After(5 * time.Second):
		t.Fatalf("the FIFO %s was not opened for writing within 5s: Inventory() never opened its read end", text.Quote(path))
	}

	t.Cleanup(func() { _ = writer.Close() })

	// Give the reader time to reach the read, which then blocks: the file is
	// open and nothing has been written to it.
	time.Sleep(50 * time.Millisecond)

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Inventory() on a FIFO whose read is blocked returned error %v, want one wrapping %v",
				err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatalf("Inventory() with a read blocked on a FIFO did not return within 1s of its context being cancelled")
	}

	// A write to a FIFO whose only reader has gone fails with EPIPE. The close
	// happens on a goroutine of its own, so the write is retried until it has;
	// an open read end keeps accepting data and never fails.
	deadline := time.Now().Add(time.Second)

	for {
		_, err := writer.Write([]byte(" "))
		if errors.Is(err, syscall.EPIPE) {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("writing to the FIFO after Inventory() was cancelled gave error %v within 1s, want %v: the file of a cancelled read must be closed",
				err, syscall.EPIPE)
		}

		time.Sleep(5 * time.Millisecond)
	}
}
