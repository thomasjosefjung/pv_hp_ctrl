package config

import (
	"encoding/json"
	"os"
)

type Config struct {
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
	Thresholds struct {
		Power float64 `json:"power"`
		Soc   float64 `json:"soc"`
	} `json:"thresholds"`
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
