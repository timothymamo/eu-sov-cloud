// Package inventoryfile reads the inventory of a source from a JSON inventory
// file, the interchange format between a future `eusov discover` and
// `eusov assess` (ADR 0006). It implements the analysis.InventorySource port and
// passes the contract suite in sourcetest.
//
// The file is untrusted input. It is read only up to MaxFileSize bytes and
// refused unless it is UTF-8 without a byte order mark. It is then read strictly:
// one JSON object with nothing after it; schemaVersion 1, found before anything
// else is checked (any other version, or none, is refused); and, token by token
// before anything is decoded, every key spelt exactly as the format defines it
// and never repeated, every value of the right JSON type, and a resources list
// (which may be empty) of at most MaxResources entries, counted as it is read.
// Only then is the document decoded. The values are validated by the domain,
// through inventory.New: an unknown category is an error, never unclassified.
//
// Errors are one line, name the file through text.Quote, and wrap ErrRead,
// ErrTooLarge, ErrDecode, ErrSchemaVersion, ErrTooManyResources, the domain's
// own sentinel or ctx.Err(). A problem in the document is placed by line and
// column. The text of OS and JSON decoder errors is never passed on: it would
// repeat the path or file contents unquoted.
//
// Cancelling the context ends a call at once, even while the file is opened or
// read: the file is opened and read on a goroutine of its own, closed on
// cancellation once it is open. An open that blocks, such as a FIFO with no
// writer, cannot be interrupted portably, so its goroutine waits until the open
// returns or the process exits.
//
// Adapter layer: reads one local file, never the network or a cloud.
package inventoryfile
