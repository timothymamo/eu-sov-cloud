// Package inventoryfile reads the inventory of a source from a JSON inventory
// file, the interchange format between a future `eusov discover` and
// `eusov assess` (ADR 0006). It implements the analysis.InventorySource port and
// passes the contract suite in sourcetest.
//
// The file is untrusted input. It is read only up to MaxFileSize bytes and
// decoded strictly: one JSON object with nothing after it, no field the format
// does not define, schemaVersion 1 (any other version, or none, is refused),
// a resources list (which may be empty) of at most MaxResources entries. The
// values are validated by the domain, through inventory.New: an unknown
// category is an error, never unclassified.
//
// Errors are one line, name the file through text.Quote, and wrap ErrRead,
// ErrTooLarge, ErrDecode, ErrSchemaVersion, ErrTooManyResources or the domain's
// own sentinel. The text of OS and JSON decoder errors is never passed on: it
// would repeat the path or file contents unquoted.
//
// Adapter layer: reads one local file, never the network or a cloud.
package inventoryfile
