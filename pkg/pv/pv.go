package pv

import (
	"encoding/json"
	"net/http"
)

type PVData struct {
	Power       float64 `json:"power"`
	Soc         float64 `json:"soc"`
	Consumption float64 `json:"consumption"`
}

type PowerResponse struct {
	Value float64 `json:"value"`
}

type SocResponse struct {
	Value float64 `json:"value"`
}

func GetData(powerURL, socURL, consumptionURL, username, password string) (*PVData, error) {
	power, err := getFloatValue(powerURL, username, password)
	if err != nil {
		return nil, err
	}

	soc, err := getFloatValue(socURL, username, password)
	if err != nil {
		return nil, err
	}

	consumption, err := getFloatValue(consumptionURL, username, password)
	if err != nil {
		return nil, err
	}

	return &PVData{Power: power, Soc: soc, Consumption: consumption}, nil
}

func getFloatValue(apiURL, username, password string) (float64, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return 0, err
	}

	req.SetBasicAuth(username, password)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var data struct {
		Value float64 `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return 0, err
	}

	return data.Value, nil
}
