// Package web membawa hasil build frontend di dalam binary server.
// Folder dist diisi oleh `make build`; saat development isinya hanya .gitkeep dan frontend dilayani Vite.
package web

import "embed"

//go:embed all:dist
var Dist embed.FS
