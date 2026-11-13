// The packages the CLI imports today, which cli-gets-adapters-from-cmd must keep
// allowing (issue #152): the CLI reads the inventory a user names (os,
// path/filepath, io/fs), parses it (encoding/json arrives through the adapters,
// which the composition root constructs), assesses it through the application
// layer and renders the report. A strict allow list says what the CLI may
// import, so an entry lost in the rewrite from $gostd fails here: this file
// carries no want comment.
//
// The module's packages are named one by one in the rule, each ending in $, so
// a package added under internal/ is no import of the CLI's until the rule names
// it; the three below are the ones the CLI imports today.
package cli

import (
	_ "bytes"
	_ "context"
	_ "errors"
	_ "flag"
	_ "fmt"
	_ "io"
	_ "io/fs"
	_ "maps"
	_ "os"
	_ "path/filepath"
	_ "slices"
	_ "strconv"
	_ "strings"
	_ "unicode"
	_ "unicode/utf8"

	_ "github.com/timothymamo/shift2eu/internal/app/analysis"
	_ "github.com/timothymamo/shift2eu/internal/domain/text"
	_ "github.com/timothymamo/shift2eu/internal/version"
)
