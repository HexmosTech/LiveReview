package docindex

import (
	"embed"
)

// DocsFS embeds all product guidance training documents under docs/
//
//go:embed docs
var DocsFS embed.FS
