package energydaemon

import (
	"testing"

	"pv_hp_ctrl/pkg/pv"
	"pv_hp_ctrl/pkg/state"
)

func TestSetEnergyStatusStoresLatestPVData(t *testing.T) {
	pvData := state.GetStatus().Energy.PVData
	if pvData != nil {
		state.SetEnergyStatus(nil)
	}

	expectedPower := 3210.0
	expectedSoc := 89.0
	state.SetEnergyStatus(&pv.PVData{Power: expectedPower, Soc: expectedSoc})

	status := state.GetStatus()
	if status.Energy.PVData == nil {
		t.Fatal("expected shared energy status to be populated")
	}

	if status.Energy.PVData.Power != expectedPower {
		t.Fatalf("Energy.PVData.Power = %v, want %v", status.Energy.PVData.Power, expectedPower)
	}

	if status.Energy.PVData.Soc != expectedSoc {
		t.Fatalf("Energy.PVData.Soc = %v, want %v", status.Energy.PVData.Soc, expectedSoc)
	}

	state.SetEnergyStatus(nil)
}
