// Command eusov-server stands in for the second composition root (PRD #103): it
// imports the standard packages the server-composition-root rule allows, and the
// same third-party library and cloud SDK its sibling cmd/eusov imports, which the
// rule must refuse. Test/architecture holds it to that rule for every platform
// the release builds, and Test/architecture's composition-root probe proves the
// composition-root rule does not hold it at all.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"example.com/logging"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:0", "address")
	concurrency := flag.Int("concurrency", 0, "assessments at once")
	flag.Parse()

	limit := debug.SetMemoryLimit(-1)
	slots := int(min(int64(max(*concurrency, runtime.NumCPU())), limit/(1<<30)))

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		hosts := slices.Sorted(slices.Values(strings.Split(*listen, ",")))
		logging.Printf("%v %v %s", os.Args, ec2.Client{}, strings.Join(hosts, ","))
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		var stderr io.Writer = os.Stderr

		slog.New(slog.NewTextHandler(stderr, nil)).Error("listen", "error", errors.Unwrap(err), "limit", limit, "at", time.Now())
		fmt.Fprintln(stderr, strconv.Itoa(slots), math.MaxInt64, http.MethodGet)

		os.Exit(1)
	}

	// The address the server would announce: the fixture never serves.
	_ = ln.Close()

	<-ctx.Done()

	wg.Wait()
}
