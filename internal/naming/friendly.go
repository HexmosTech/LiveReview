package naming

import (
	crand "crypto/rand"
	"fmt"
	"math/big"
	"strings"
)

var friendlyAdjectives = []string{
	"amber",
	"azure",
	"bold",
	"brave",
	"bright",
	"calm",
	"clear",
	"cool",
	"crystal",
	"dynamic",
	"elastic",
	"emerald",
	"epic",
	"flame",
	"fluid",
	"frost",
	"gentle",
	"golden",
	"horizon",
	"iron",
	"jade",
	"keen",
	"lunar",
	"mint",
	"mystic",
	"nimble",
	"nova",
	"pearl",
	"phantom",
	"prism",
	"quantum",
	"radiant",
	"ruby",
	"shadow",
	"silent",
	"silver",
	"skyline",
	"solar",
	"swift",
	"thunder",
	"tranquil",
	"turbo",
	"velvet",
	"vibrant",
	"vital",
	"wild",
	"zenith",
}

var friendlyNouns = []string{
	"anchor",
	"arrow",
	"atlas",
	"beacon",
	"blaze",
	"cascade",
	"citadel",
	"cloud",
	"comet",
	"compass",
	"crystal",
	"delta",
	"echo",
	"ember",
	"falcon",
	"flame",
	"forge",
	"frontier",
	"galaxy",
	"harbor",
	"horizon",
	"insight",
	"journey",
	"keystone",
	"lambda",
	"legend",
	"meridian",
	"nebula",
	"nexus",
	"orbit",
	"peak",
	"phoenix",
	"pinnacle",
	"pixel",
	"prism",
	"pulse",
	"quartz",
	"quest",
	"rapids",
	"ray",
	"reef",
	"ridge",
	"sage",
	"scout",
	"sentinel",
	"shadow",
	"signal",
	"spark",
	"spectrum",
	"sphere",
	"summit",
	"tide",
	"torch",
	"tower",
	"vector",
	"vertex",
	"vista",
	"vortex",
	"wave",
	"zenith",
}

// GenerateFriendlyName generates a random docker-style friendly name combining an adjective and noun
func GenerateFriendlyName() string {
	adj := pickRandomWord(friendlyAdjectives)
	noun := pickRandomWord(friendlyNouns)

	switch {
	case adj == "" && noun == "":
		return "Untitled Run"
	case noun == "":
		return capitalize(adj)
	case adj == "":
		return capitalize(noun)
	default:
		return fmt.Sprintf("%s %s", capitalize(adj), capitalize(noun))
	}
}

func pickRandomWord(options []string) string {
	if len(options) == 0 {
		return ""
	}
	max := big.NewInt(int64(len(options)))
	idx, err := crand.Int(crand.Reader, max)
	if err != nil {
		return options[0]
	}
	return options[idx.Int64()]
}

func capitalize(word string) string {
	if len(word) == 0 {
		return ""
	}
	if len(word) == 1 {
		return strings.ToUpper(word)
	}
	return strings.ToUpper(word[:1]) + word[1:]
}
