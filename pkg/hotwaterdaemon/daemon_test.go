package hotwaterdaemon

import (
	"testing"
	"time"

	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/state"
)

type fakeDisableClient struct {
	extraHotWaterActive bool
	temperature         float64
	setCalls            []bool
	setTemperatureCalls []float64
	setErr              error
}

func (c *fakeDisableClient) GetExtraHotWater(string) (bool, error) {
	return c.extraHotWaterActive, nil
}

func (c *fakeDisableClient) SetExtraHotWater(_ string, enabled bool) error {
	c.setCalls = append(c.setCalls, enabled)
	return c.setErr
}

func (c *fakeDisableClient) SetExtraHotWaterTemperature(_ string, value float64) error {
	c.setTemperatureCalls = append(c.setTemperatureCalls, value)
	return c.setErr
}

func (c *fakeDisableClient) GetDomesticWaterTemperature(string) (float64, error) {
	return c.temperature, nil
}

func TestEnergyConditionsMet(t *testing.T) {
	t.Run("requires pv power above threshold", func(t *testing.T) {
		if energyConditionsMet(5000, 96, 5000, 95) {
			t.Fatal("expected power at threshold to remain off")
		}

		if !energyConditionsMet(5001, 96, 5000, 95) {
			t.Fatal("expected power above threshold to satisfy condition")
		}
	})

	t.Run("keeps soc threshold enforced", func(t *testing.T) {
		if energyConditionsMet(5500, 95, 5000, 95) {
			t.Fatal("expected SOC at threshold to remain off")
		}

		if !energyConditionsMet(5500, 96, 5000, 95) {
			t.Fatal("expected SOC above threshold to satisfy condition")
		}
	})
}

func TestKeepExtraHotWaterActive(t *testing.T) {
	t.Run("requires pv power above threshold", func(t *testing.T) {
		if keepExtraHotWaterActive(5000, 5000) {
			t.Fatal("expected power at threshold to stop keeping extra hot water active")
		}

		if !keepExtraHotWaterActive(5001, 5000) {
			t.Fatal("expected power above threshold to keep extra hot water active")
		}
	})
}

func TestPowerThreshold(t *testing.T) {
	t.Run("uses configured power threshold when present", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.ThresholdsHotWater.Power = 6200

		if got := cfg.HotWaterPowerThreshold(); got != 6200 {
			t.Fatalf("HotWaterPowerThreshold() = %v, want %v", got, 6200)
		}
	})

	t.Run("uses default when nothing configured", func(t *testing.T) {
		cfg := &config.Config{}

		if got := cfg.HotWaterPowerThreshold(); got != 5000 {
			t.Fatalf("HotWaterPowerThreshold() = %v, want %v", got, 5000)
		}
	})
}

func TestHotWaterSwitchOnHysteresisDuration(t *testing.T) {
	t.Run("uses dedicated switch-on value when configured", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.ThresholdsHotWater.SwitchOnHysteresisMinutes = 5

		if got := cfg.HotWaterSwitchOnHysteresisDuration(); got != 5*time.Minute {
			t.Fatalf("HotWaterSwitchOnHysteresisDuration() = %v, want %v", got, 5*time.Minute)
		}
	})

	t.Run("uses default when nothing configured", func(t *testing.T) {
		cfg := &config.Config{}

		if got := cfg.HotWaterSwitchOnHysteresisDuration(); got != 5*time.Minute {
			t.Fatalf("HotWaterSwitchOnHysteresisDuration() = %v, want %v", got, 5*time.Minute)
		}
	})
}

func TestHotWaterSwitchOffHysteresisDuration(t *testing.T) {
	t.Run("uses dedicated switch-off value when configured", func(t *testing.T) {
		cfg := &config.Config{}
		cfg.ThresholdsHotWater.SwitchOffHysteresisMinutes = 10

		if got := cfg.HotWaterSwitchOffHysteresisDuration(); got != 10*time.Minute {
			t.Fatalf("HotWaterSwitchOffHysteresisDuration() = %v, want %v", got, 10*time.Minute)
		}
	})

	t.Run("uses default when nothing configured", func(t *testing.T) {
		cfg := &config.Config{}

		if got := cfg.HotWaterSwitchOffHysteresisDuration(); got != 10*time.Minute {
			t.Fatalf("HotWaterSwitchOffHysteresisDuration() = %v, want %v", got, 10*time.Minute)
		}
	})
}

func TestDisableWithClientPreservesExtraHotWaterState(t *testing.T) {
	tests := []struct {
		name                string
		extraHotWaterActive bool
	}{
		{name: "keeps active state", extraHotWaterActive: true},
		{name: "keeps inactive state", extraHotWaterActive: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.MyUplink.DeviceID = "device-1"
			client := &fakeDisableClient{extraHotWaterActive: test.extraHotWaterActive, temperature: 48.5}

			if err := disableWithClient(cfg, client, "disabled"); err != nil {
				t.Fatalf("disableWithClient() error = %v", err)
			}

			if len(client.setCalls) != 0 {
				t.Fatalf("SetExtraHotWater() calls = %v, want no calls", client.setCalls)
			}

			status := state.GetStatus().HotWater
			if status.IsActive {
				t.Fatal("expected hot-water daemon status to be inactive")
			}

			if status.ExtraHotWaterActive != test.extraHotWaterActive {
				t.Fatalf("ExtraHotWaterActive = %v, want %v", status.ExtraHotWaterActive, test.extraHotWaterActive)
			}

			if status.Message != "disabled" {
				t.Fatalf("status message = %q, want %q", status.Message, "disabled")
			}
		})
	}
}

func TestHandleActiveExtraHotWater(t *testing.T) {
	originalTimingState := timingState
	defer func() {
		timingState = originalTimingState
	}()

	temperature := 49.2

	t.Run("keeps active status when pv remains high", func(t *testing.T) {
		timingState.ConditionsMetSince = time.Now().Add(-time.Minute)
		timingState.ConditionsNotMetSince = time.Now().Add(-time.Minute)

		handleActiveExtraHotWater(6000, 98, 5000, &temperature)

		status := state.GetStatus().HotWater
		if !status.IsActive {
			t.Fatal("expected hot-water daemon status to remain active")
		}

		if !status.ExtraHotWaterActive {
			t.Fatal("expected extra hot water to remain active")
		}

		if status.Message != "Extra-WW aktiv" {
			t.Fatalf("status message = %q, want %q", status.Message, "Extra-WW aktiv")
		}

		if timingState.ConditionsMetSince != (time.Time{}) {
			t.Fatal("expected switch-on hysteresis timestamp to be cleared")
		}

		if timingState.ConditionsNotMetSince != (time.Time{}) {
			t.Fatal("expected switch-off hysteresis timestamp to be cleared")
		}
	})

	t.Run("keeps active status when pv drops", func(t *testing.T) {
		timingState.ConditionsMetSince = time.Now().Add(-time.Minute)
		timingState.ConditionsNotMetSince = time.Now().Add(-time.Minute)

		handleActiveExtraHotWater(1000, 98, 5000, &temperature)

		status := state.GetStatus().HotWater
		if !status.IsActive {
			t.Fatal("expected hot-water daemon status to remain active")
		}

		if !status.ExtraHotWaterActive {
			t.Fatal("expected extra hot water to remain active")
		}

		if status.Message != "Extra-WW aktiv, Abschaltung erfolgt nur durch Heizungssystem" {
			t.Fatalf("status message = %q, want %q", status.Message, "Extra-WW aktiv, Abschaltung erfolgt nur durch Heizungssystem")
		}

		if status.SwitchOffHysteresis.Active {
			t.Fatal("expected switch-off hysteresis to stay inactive")
		}

		if timingState.ConditionsMetSince != (time.Time{}) {
			t.Fatal("expected switch-on hysteresis timestamp to be cleared")
		}

		if timingState.ConditionsNotMetSince != (time.Time{}) {
			t.Fatal("expected switch-off hysteresis timestamp to be cleared")
		}
	})
}
