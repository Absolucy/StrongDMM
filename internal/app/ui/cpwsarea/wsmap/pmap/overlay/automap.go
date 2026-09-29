package overlay

import "sdmm/internal/util"

// Picked by template position, so a template's outline and its row in the settings panel match up.
var automapColors = []util.Color{
	util.MakeColor(0.35, 0.85, 1, 1),
	util.MakeColor(1, 0.75, 0.2, 1),
	util.MakeColor(0.65, 1, 0.35, 1),
	util.MakeColor(1, 0.45, 0.85, 1),
	util.MakeColor(0.6, 0.55, 1, 1),
	util.MakeColor(0.3, 1, 0.8, 1),
	util.MakeColor(1, 1, 0.45, 1),
	util.MakeColor(1, 0.6, 0.45, 1),
}

var (
	ColorAutomapFrozenFill   = util.MakeColor(1, 0.2, 0.2, 0.35)
	ColorAutomapFrozenBorder = util.MakeColor(1, 0.2, 0.2, 1)
)

func ColorAutomapTemplate(templateIdx int) util.Color {
	return automapColors[templateIdx%len(automapColors)]
}
