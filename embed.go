package main

import "embed"

//go:embed all:frontend
var frontendAssets embed.FS

//go:embed assets
var bundledCatalogueFS embed.FS
