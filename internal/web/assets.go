package web

import "embed"

//go:embed static/index.html static/style.css static/app.js
var staticFiles embed.FS
