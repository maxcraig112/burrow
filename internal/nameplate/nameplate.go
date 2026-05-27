package nameplate

import (
	"fmt"
	"math/rand/v2"
)

// ~50 words per category gives 50^3 = 125,000 combinations.
var adjectives = []string{
	"amber", "brisk", "calm", "damp", "eager",
	"faint", "grand", "hazy", "idle", "jolly",
	"keen", "lush", "muted", "noble", "opal",
	"prime", "quiet", "rapid", "swift", "tawny",
	"ultra", "vivid", "warm", "xenial", "young",
	"zesty", "arctic", "bold", "crisp", "dusty",
	"early", "frosty", "gauzy", "hollow", "inky",
	"jade", "kinetic", "lunar", "misty", "neon",
	"obsidian", "pale", "rosy", "silver", "twilight",
	"urban", "velvet", "wispy", "xeric", "yellow",
}

var nouns = []string{
	"anchor", "bridge", "cedar", "delta", "ember",
	"fjord", "grove", "harbor", "inlet", "jasper",
	"kettle", "lantern", "marsh", "needle", "orbit",
	"pebble", "quarry", "ridge", "shoal", "thorn",
	"umber", "valley", "willow", "xenon", "yarrow",
	"zenith", "basin", "canopy", "dune", "estuary",
	"flint", "glacier", "hollow", "islet", "juniper",
	"kelp", "ledge", "mesa", "nave", "outcrop",
	"pinnacle", "reef", "summit", "tundra", "uplift",
	"vortex", "waypoint", "xyster", "yew", "zephyr",
}

var verbs = []string{
	"blooms", "carves", "drifts", "echoes", "flows",
	"glows", "hums", "idles", "jumps", "keeps",
	"leaps", "melts", "nods", "orbits", "pulses",
	"quiets", "rises", "sings", "turns", "unfolds",
	"veers", "wades", "yields", "zips", "arcs",
	"bends", "climbs", "dives", "erodes", "fades",
	"glides", "hovers", "ignites", "juts", "kindles",
	"lingers", "moves", "nudges", "opens", "passes",
	"recedes", "settles", "tilts", "unfurls", "ventures",
	"wanders", "expands", "yearns", "zooms", "soars",
}

// Generate returns a random nameplate in the form "adjective-noun-verb".
func Generate() string {
	return fmt.Sprintf("%s-%s-%s",
		adjectives[rand.IntN(len(adjectives))],
		nouns[rand.IntN(len(nouns))],
		verbs[rand.IntN(len(verbs))],
	)
}
