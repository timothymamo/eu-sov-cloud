// Command eusov analyses infrastructure running in a hyperscaler (AWS, Azure,
// GCP) and reports what can move to an EU cloud provider (OVHcloud, Scaleway,
// UpCloud).
//
// main is the composition root: it is the only place that wires concrete
// adapters into the application use cases.
package main

import (
	"fmt"
	"os"

	"github.com/timothymamo/eu-sov-cloud/internal/version"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "version" || os.Args[1] == "--version") {
		fmt.Println(version.String())
		return
	}

	fmt.Fprintln(os.Stderr, "eusov: analysis commands are not implemented yet; try `eusov version`")
	os.Exit(2)
}
