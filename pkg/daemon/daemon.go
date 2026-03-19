package daemon

import (
	"fmt"
	"log"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/myuplink"
	"pv_hp_ctrl/pkg/pv"
	"pv_hp_ctrl/pkg/state"
	"time"
)

func Run() {
	// Run once at the beginning
	checkAndControl()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		checkAndControl()
	}
}

func checkAndControl() {
	cfg, err := config.Load("config.json")
	if err != nil {
		log.Printf("Failed to load config: %v", err)
		state.SetStatus(fmt.Sprintf("Error: %v", err), nil, false)
		return
	}

	pvData, err := pv.GetData(cfg.PV.PowerURL, cfg.PV.SocURL, cfg.PV.ConsumptionURL, cfg.PV.Username, cfg.PV.Password)
	if err != nil {
		log.Printf("Failed to get PV data: %v", err)
		state.SetStatus(fmt.Sprintf("Error: %v", err), nil, false)
		return
	}

	if (pvData.Power-pvData.Consumption) > cfg.Thresholds.Power &&
		pvData.Soc > cfg.Thresholds.Soc &&
		!state.GetStatus().IsActive &&
		time.Now().Hour() < 13 {
		// if true {
		authCode := "your-manual-auth-code" // IMPORTANT: Replace this

		token, err := myuplink.GetAccessToken(cfg.MyUplink.ClientID, cfg.MyUplink.ClientSecret, cfg.MyUplink.RedirectURI, authCode)
		if err != nil {
			log.Printf("Failed to get myUplink access token: %v", err)
			state.SetStatus(fmt.Sprintf("Error: %v", err), pvData, false)
			return
		}

		err = myuplink.SetExtraWarmWater(cfg.MyUplink.DeviceID, token.AccessToken)
		if err != nil {
			log.Printf("Failed to set extra warm water: %v", err)
			state.SetStatus(fmt.Sprintf("Error: %v", err), pvData, false)
			return
		}

		state.SetStatus("Extra warm water activated", pvData, true)
		log.Println("Extra warm water activated")
		return
	}

	state.SetStatus("Conditions not met for extra warm water", pvData, false)
	log.Println("Conditions not met for extra warm water")
}
