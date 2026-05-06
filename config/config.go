package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultPath                  = "config.json"
	defaultPowerThreshold        = 5000
	defaultSocThreshold          = 95
	defaultSwitchOnHysteresis    = 5 * time.Minute
	defaultSwitchOffHysteresis   = 10 * time.Minute
	defaultActivationCutoffHour  = 14
	defaultHeatingPowerThreshold = 5000
	defaultHeatingSocThreshold   = 95
	defaultHeatingNormalOffset   = -1.0
	defaultHeatingPVOffset       = 1.0
)

type Config struct {
	Daemons struct {
		HotWaterEnabled *bool `json:"hot_water_enabled,omitempty"`
		HeatingEnabled  *bool `json:"heating_enabled,omitempty"`
	} `json:"daemons,omitempty"`
	PV struct {
		PowerURL       string `json:"power_url"`
		SocURL         string `json:"soc_url"`
		ConsumptionURL string `json:"consumption_url"`
		Username       string `json:"username"`
		Password       string `json:"password"`
	} `json:"pv"`
	MyUplink struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		RedirectURI  string `json:"redirect_uri"`
		DeviceID     string `json:"device_id"`
	} `json:"myuplink"`
	ThresholdsHotWater struct {
		Power                      float64 `json:"power"`
		Soc                        float64 `json:"soc"`
		SwitchOnHysteresisMinutes  int     `json:"switch_on_hysteresis_minutes"`
		SwitchOffHysteresisMinutes int     `json:"switch_off_hysteresis_minutes"`
		ActivationCutoff           int     `json:"activation_cutoff"`
	} `json:"thresholds_hot_water"`
	ThresholdsHeating struct {
		Power                      float64  `json:"power"`
		Soc                        float64  `json:"soc"`
		SwitchOnHysteresisMinutes  int      `json:"switch_on_hysteresis_minutes"`
		SwitchOffHysteresisMinutes int      `json:"switch_off_hysteresis_minutes"`
		NormalOffset               *float64 `json:"normal_offset"`
		PVOffset                   *float64 `json:"pv_offset"`
	} `json:"thresholds_heating"`
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	if err := json.NewDecoder(file).Decode(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(c); err != nil {
		return err
	}

	if err := file.Sync(); err != nil {
		return err
	}

	return nil
}

func (c *Config) HotWaterDaemonEnabled() bool {
	if c == nil || c.Daemons.HotWaterEnabled == nil {
		return true
	}

	return *c.Daemons.HotWaterEnabled
}

func (c *Config) HeatingDaemonEnabled() bool {
	if c == nil || c.Daemons.HeatingEnabled == nil {
		return true
	}

	return *c.Daemons.HeatingEnabled
}

func (c *Config) SetHotWaterDaemonEnabled(enabled bool) {
	c.Daemons.HotWaterEnabled = &enabled
}

func (c *Config) SetHeatingDaemonEnabled(enabled bool) {
	c.Daemons.HeatingEnabled = &enabled
}

func (c *Config) HotWaterSwitchOnHysteresisDuration() time.Duration {
	if c != nil && c.ThresholdsHotWater.SwitchOnHysteresisMinutes > 0 {
		return time.Duration(c.ThresholdsHotWater.SwitchOnHysteresisMinutes) * time.Minute
	}

	return defaultSwitchOnHysteresis
}

func (c *Config) HotWaterSwitchOffHysteresisDuration() time.Duration {
	if c != nil && c.ThresholdsHotWater.SwitchOffHysteresisMinutes > 0 {
		return time.Duration(c.ThresholdsHotWater.SwitchOffHysteresisMinutes) * time.Minute
	}

	return defaultSwitchOffHysteresis
}

func (c *Config) HotWaterPowerThreshold() float64 {
	if c != nil && c.ThresholdsHotWater.Power > 0 {
		return c.ThresholdsHotWater.Power
	}

	return defaultPowerThreshold
}

func (c *Config) HotWaterSocThreshold() float64 {
	if c != nil && c.ThresholdsHotWater.Soc > 0 {
		return c.ThresholdsHotWater.Soc
	}

	return defaultSocThreshold
}

func (c *Config) HotWaterActivationCutoffHour() int {
	if c != nil && c.ThresholdsHotWater.ActivationCutoff > 0 {
		return c.ThresholdsHotWater.ActivationCutoff
	}

	return defaultActivationCutoffHour
}

func (c *Config) HeatingPowerThreshold() float64 {
	if c != nil && c.ThresholdsHeating.Power > 0 {
		return c.ThresholdsHeating.Power
	}

	return defaultHeatingPowerThreshold
}

func (c *Config) HeatingSocThreshold() float64 {
	if c != nil && c.ThresholdsHeating.Soc > 0 {
		return c.ThresholdsHeating.Soc
	}

	return defaultHeatingSocThreshold
}

func (c *Config) HeatingSwitchOnHysteresisDuration() time.Duration {
	if c != nil && c.ThresholdsHeating.SwitchOnHysteresisMinutes > 0 {
		return time.Duration(c.ThresholdsHeating.SwitchOnHysteresisMinutes) * time.Minute
	}

	return defaultSwitchOnHysteresis
}

func (c *Config) HeatingSwitchOffHysteresisDuration() time.Duration {
	if c != nil && c.ThresholdsHeating.SwitchOffHysteresisMinutes > 0 {
		return time.Duration(c.ThresholdsHeating.SwitchOffHysteresisMinutes) * time.Minute
	}

	return defaultSwitchOffHysteresis
}

func (c *Config) HeatingNormalOffset() float64 {
	if c != nil && c.ThresholdsHeating.NormalOffset != nil {
		return *c.ThresholdsHeating.NormalOffset
	}

	return defaultHeatingNormalOffset
}

func (c *Config) HeatingPVOffset() float64 {
	if c != nil && c.ThresholdsHeating.PVOffset != nil {
		return *c.ThresholdsHeating.PVOffset
	}

	return defaultHeatingPVOffset
}
