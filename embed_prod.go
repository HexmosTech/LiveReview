//go:build !ci

package main

import "embed"

//go:embed ui/dist/*
var embeddedFiles embed.FS