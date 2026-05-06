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
	setErr              error
}

func (c *fakeDisableClient) GetExtraHotWater(string) (bool, error) {
	return c.extraHotWaterActive, nil
}

func (c *fakeDisableClient) SetExtraHotWater(_ string, enabled bool) error {
	c.setCalls = append(c.setCalls, enabled)
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

func TestDisableWithClientForcesExtraHotWaterOff(t *testing.T) {
	cfg := &config.Config{}
	cfg.MyUplink.DeviceID = "device-1"
	client := &fakeDisableClient{extraHotWaterActive: true, temperature: 48.5}

	if err := disableWithClient(cfg, client, "disabled"); err != nil {
		t.Fatalf("disableWithClient() error = %v", err)
	}

	if len(client.setCalls) != 1 || client.setCalls[0] {
		t.Fatalf("SetExtraHotWater() calls = %v, want [false]", client.setCalls)
	}

	status := state.GetStatus().HotWater
	if status.IsActive {
		t.Fatal("expected hot-water daemon status to be inactive")
	}

	if status.ExtraHotWaterActive {
		t.Fatal("expected extra hot water to be marked inactive")
	}

	if status.Message != "disabled" {
		t.Fatalf("status message = %q, want %q", status.Message, "disabled")
	}
}
