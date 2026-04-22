package heatingdaemon

import (
	"testing"
	"time"

	"pv_hp_ctrl/config"
)

func TestHeatingConditionsMet(t *testing.T) {
	t.Run("requires pv power above threshold", func(t *testing.T) {
		if heatingConditionsMet(4500, 96, 4500, 95) {
			t.Fatal("expected heating offset to remain off at power threshold")
		}

		if !heatingConditionsMet(4501, 96, 4500, 95) {
			t.Fatal("expected heating offset to activate above power threshold")
		}
	})

	t.Run("requires soc above threshold", func(t *testing.T) {
		if heatingConditionsMet(4501, 95, 4500, 95) {
			t.Fatal("expected heating offset to remain off at SOC threshold")
		}

		if !heatingConditionsMet(4501, 96, 4500, 95) {
			t.Fatal("expected heating offset to activate above SOC threshold")
		}
	})
}

func TestHeatingConfigDefaultsAndOverrides(t *testing.T) {
	t.Run("uses configured values when present", func(t *testing.T) {
		cfg := &config.Config{}
		normalOffset := -2.0
		pvOffset := 1.5
		cfg.ThresholdsHeating.Power = 4300
		cfg.ThresholdsHeating.Soc = 88
		cfg.ThresholdsHeating.SwitchOnHysteresisMinutes = 7
		cfg.ThresholdsHeating.SwitchOffHysteresisMinutes = 12
		cfg.ThresholdsHeating.NormalOffset = &normalOffset
		cfg.ThresholdsHeating.PVOffset = &pvOffset

		if got := cfg.HeatingPowerThreshold(); got != 4300 {
			t.Fatalf("HeatingPowerThreshold() = %v, want %v", got, 4300)
		}

		if got := cfg.HeatingSocThreshold(); got != 88 {
			t.Fatalf("HeatingSocThreshold() = %v, want %v", got, 88)
		}

		if got := cfg.HeatingSwitchOnHysteresisDuration(); got != 7*time.Minute {
			t.Fatalf("HeatingSwitchOnHysteresisDuration() = %v, want %v", got, 7*time.Minute)
		}

		if got := cfg.HeatingSwitchOffHysteresisDuration(); got != 12*time.Minute {
			t.Fatalf("HeatingSwitchOffHysteresisDuration() = %v, want %v", got, 12*time.Minute)
		}

		if got := cfg.HeatingNormalOffset(); got != -2.0 {
			t.Fatalf("HeatingNormalOffset() = %v, want %v", got, -2.0)
		}

		if got := cfg.HeatingPVOffset(); got != 1.5 {
			t.Fatalf("HeatingPVOffset() = %v, want %v", got, 1.5)
		}
	})

	t.Run("uses defaults when not configured", func(t *testing.T) {
		cfg := &config.Config{}

		if got := cfg.HeatingPowerThreshold(); got != 5000 {
			t.Fatalf("HeatingPowerThreshold() = %v, want %v", got, 5000)
		}

		if got := cfg.HeatingSocThreshold(); got != 95 {
			t.Fatalf("HeatingSocThreshold() = %v, want %v", got, 95)
		}

		if got := cfg.HeatingSwitchOnHysteresisDuration(); got != 5*time.Minute {
			t.Fatalf("HeatingSwitchOnHysteresisDuration() = %v, want %v", got, 5*time.Minute)
		}

		if got := cfg.HeatingSwitchOffHysteresisDuration(); got != 10*time.Minute {
			t.Fatalf("HeatingSwitchOffHysteresisDuration() = %v, want %v", got, 10*time.Minute)
		}

		if got := cfg.HeatingNormalOffset(); got != -1.0 {
			t.Fatalf("HeatingNormalOffset() = %v, want %v", got, -1.0)
		}

		if got := cfg.HeatingPVOffset(); got != 1.0 {
			t.Fatalf("HeatingPVOffset() = %v, want %v", got, 1.0)
		}
	})
}
