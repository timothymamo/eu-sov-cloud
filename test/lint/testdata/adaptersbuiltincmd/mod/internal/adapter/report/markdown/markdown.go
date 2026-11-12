// Package markdown stands for a report adapter. It imports a helper under
// adapters/internal/ that adapters-are-built-in-cmd does not name, so the rule
// must report it with the helper message; reporters-stay-offline refuses the
// helper too, which the want on the line names as well.
package markdown

import "github.com/timothymamo/shift2eu/internal/adapter/internal/newhelper" // want "import 'github.com/timothymamo/shift2eu/internal/adapter/internal/newhelper' is not allowed from list 'adapters-are-built-in-cmd': an adapter imports only the shared helpers named in adapters-are-built-in-cmd" "import 'github.com/timothymamo/shift2eu/internal/adapter/internal/newhelper' is not allowed from list 'reporters-stay-offline'"

// Name names the renderer.
const Name = newhelper.Name + " markdown"
