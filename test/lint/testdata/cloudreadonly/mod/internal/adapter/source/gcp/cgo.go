package gcp

import "C" // want "import 'unsafe' is not allowed from list 'cloud-adapters-reach-the-cloud-through-the-sdk'"

// C code can run a program or open a connection no lint rule sees. depguard
// sees the imports of the code cgo generates, not C itself, so cgo is refused
// through unsafe. A comment right above import "C" would be its C preamble.
var _ = C.int(0)
