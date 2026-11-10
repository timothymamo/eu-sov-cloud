//go:build unix

package inventoryfile_test

import (
	"context"
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/timothymamo/eu-sov-cloud/internal/adapter/source/inventoryfile"
	"github.com/timothymamo/eu-sov-cloud/internal/domain/text"
)

// TestInventoryReturnsWhenTheFIFOHasNoWriter pins that a read that would block
// still ends when the context is cancelled: opening a FIFO waits for a writer
// that never comes, so only a context-aware open returns at all.
//
// The test is Unix-only, since it needs syscall.Mkfifo, and not parallel, since
// it measures how long the call takes.
func TestInventoryReturnsWhenTheFIFOHasNoWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fifo.json")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatalf("creating the FIFO %s: %v", text.Quote(path), err)
	}

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
