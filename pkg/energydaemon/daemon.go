package energydaemon

import (
	"log"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/daemoncore"
	"pv_hp_ctrl/pkg/state"
)

func RunTask() {
	pvData, err := daemoncore.LoadEnergyData(config.DefaultPath)
	if err != nil {
		log.Printf("Failed to load shared energy status: %v", err)
		state.SetEnergyStatus(nil)
		return
	}

	state.SetEnergyStatus(pvData)
}
