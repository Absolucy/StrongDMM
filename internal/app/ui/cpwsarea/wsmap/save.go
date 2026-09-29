package wsmap

import (
	"sdmm/internal/app/prefs"
	"sdmm/internal/dmapi/dmmsave"
	"sdmm/internal/util"

	"github.com/rs/zerolog/log"
)

func (ws *WsMap) Save() bool {
	log.Print("saving map workspace:", ws.CommandStackId())

	editorPrefs := ws.app.Prefs().Editor

	var saveFormat dmmsave.Format
	switch editorPrefs.SaveFormat {
	case prefs.SaveFormatInitial:
		saveFormat = dmmsave.FormatInitial
	case prefs.SaveFormatTGM:
		saveFormat = dmmsave.FormatTGM
	case prefs.SaveFormatDMM:
		saveFormat = dmmsave.FormatDM
	}

	cfg := dmmsave.Config{
		Format:            saveFormat,
		SanitizeVariables: editorPrefs.SanitizeVariables,
	}

	var err error
	if layer := ws.paneMap.Layer(); layer != nil {
		err = layer.Save(ws.app.LoadedEnvironment(), ws.paneMap.Dmm(), cfg)
	} else {
		err = dmmsave.Save(ws.app.LoadedEnvironment(), ws.paneMap.Dmm(), cfg)
	}
	if err != nil {
		log.Print("unable to save the map:", err)
		util.ShowErrorDialog("Unable to save the map: " + err.Error())
		return false
	}

	ws.app.CommandStorage().ForceBalance(ws.CommandStackId())
	return true
}
