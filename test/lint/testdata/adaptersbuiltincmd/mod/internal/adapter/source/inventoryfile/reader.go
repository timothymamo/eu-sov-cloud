// Package inventoryfile stands for the inventory file source adapter. It
// imports the shared JSON decoder, which adapters-are-built-in-cmd and
// inventory-file-reader-reads-one-file both allow, so it is not reported.
package inventoryfile

import "github.com/timothymamo/shift2eu/internal/adapter/internal/jsondoc"

// Name names the reader.
const Name = jsondoc.Name + " inventoryfile"
