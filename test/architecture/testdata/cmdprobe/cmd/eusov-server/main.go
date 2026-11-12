// Command eusov-server stands in for a sibling of the composition root (PRD
// #103): a rule for it is written the same way, so this one must not hold it. It
// imports the same third-party library the probe's cmd/eusov imports, which
// Test/architecture proves the composition-root rule does not refuse here.
package main

import "example.com/logging"

func main() {
	logging.Printf("sibling of the composition root")
}
