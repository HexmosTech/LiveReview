//go:build !ci

package main

import "embed"

//go:embed ui/dist/*
var uiAssets embed.FS