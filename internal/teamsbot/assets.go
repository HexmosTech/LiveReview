package teamsbot

import _ "embed"

// teamsAppColorPNG is the 192x192 full-color Livi icon required by the
// Teams app manifest's icons.color field.
//
//go:embed assets/color.png
var teamsAppColorPNG []byte

// teamsAppOutlinePNG is the 32x32 transparent white-silhouette Livi icon
// required by the Teams app manifest's icons.outline field.
//
//go:embed assets/outline.png
var teamsAppOutlinePNG []byte
