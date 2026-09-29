package dmmsave

import (
	"fmt"

	"sdmm/internal/dmapi/dmenv"

	"sdmm/internal/dmapi/dmmap"

	"github.com/rs/zerolog/log"
)

func Save(dme *dmenv.Dme, dmm *dmmap.Dmm, cfg Config) error {
	return SaveV(dme, dmm, dmm.Path.Absolute, cfg)
}

func SaveV(dme *dmenv.Dme, dmm *dmmap.Dmm, path string, cfg Config) error {
	log.Printf("save started [%s]...", path)

	sp, err := makeSaveProcess(cfg, dme, dmm, path)
	if err != nil {
		return fmt.Errorf("starting save process: %w", err)
	}

	if cfg.SanitizeVariables {
		sp.sanitizeVariables()
	}

	sp.handleReusedKeys()
	if err = sp.handleLocationsWithoutKeys(); err != nil {
		return fmt.Errorf("handling locations without keys: %w", err)
	}
	if err = sp.output.Save(); err != nil {
		return err
	}

	log.Print("save finished")
	return nil
}
