// Package inventoryfile reads the inventory of a source from a JSON inventory
// file, the interchange format between a future `eusov discover` and
// `eusov assess` (ADR 0006). It implements the analysis.InventorySource port and
// passes the contract suite in sourcetest.
//
// The file is untrusted input. It is read only up to MaxFileSize bytes and
// refused unless it is UTF-8 without a byte order mark. It is then read strictly:
// one JSON object with nothing after it; schemaVersion 1, found before any key
// or type is checked (any other version, or none, is refused); and, token by
// token before anything is decoded, every key spelt exactly as the format
// defines it, character for character once JSON escapes are decoded, and never
// repeated, every value of the right JSON type, no string that escapes a lone
// surrogate, a resources list (which may be empty) of at most MaxResources
// entries, and at most MaxCapabilities capabilities in each, counted as they
// are read. Only then is the document decoded. The values are validated by the
// domain, through inventory.New: an unknown category is an error, never
// unclassified.
//
// Errors are one line, name the file through text.Quote, and wrap ErrRead,
// ErrTooLarge, ErrDecode, ErrSchemaVersion, ErrTooManyResources, the domain's
// own sentinel or ctx.Err(). A problem in the document is placed by line and
// column, and a key or value is named by its path, resources[1].region. The
// text of OS and JSON decoder errors is never passed on: it would repeat the
// path or file contents unquoted.
//
// Cancelling the context ends a call at once, even while the file is opened or
// read: the file is opened and read on a goroutine of its own, closed on
// cancellation once it is open, which ends a read blocked on a pipe or FIFO. An
// open that blocks, such as a FIFO with no writer, or a read Go cannot poll,
// such as one on a stalled network file system, cannot be interrupted portably,
// so its goroutine waits until it returns or the process exits.
//
// Adapter layer: reads one local file, never the network or a cloud.
package inventoryfile
