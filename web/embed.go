package web

import "embed"

//go:embed dist index.html
var FS embed.FS
