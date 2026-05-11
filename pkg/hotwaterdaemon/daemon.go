package hotwaterdaemon

import (
	"fmt"
	"log"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/daemoncore"
	"pv_hp_ctrl/pkg/myuplink"
	"pv_hp_ctrl/pkg/state"
	"time"
)

var (
	clientProvider = &daemoncore.ClientProvider{}
	// timingState holds the hysteresis timestamps between polling cycles.
	//
	// This is the Go version of keeping state in a shared service object instead
	// of a base class field: the daemon composes a small reusable helper type.
	timingState = daemoncore.TimingState{}
)

func RunTask() {
	deps, err := daemoncore.LoadDependencies(config.DefaultPath, clientProvider)
	if err != nil {
		log.Printf("Failed to load daemon dependencies: %v", err)
		state.SetHotWaterStatus(fmt.Sprintf("Error: %v", err), false, false, nil, state.HysteresisStatus{}, state.HysteresisStatus{})
		return
	}

	cfg := deps.Config
	client := deps.Client
	if !cfg.HotWaterDaemonEnabled() {
		if err := disableWithClient(cfg, client, "WW-Daemon deaktiviert"); err != nil {
			log.Printf("Failed to refresh extra hot water status while daemon disabled: %v", err)
		}
		return
	}

	pvData := state.GetStatus().Energy.PVData
	if pvData == nil {
		state.SetHotWaterStatus("Energiedaten fehlen", false, false, nil, state.HysteresisStatus{}, state.HysteresisStatus{})
		return
	}

	extraHotWaterActive, err := client.GetExtraHotWater(cfg.MyUplink.DeviceID)
	if err != nil {
		log.Printf("Failed to get extra hot water status: %v", err)
		state.SetHotWaterStatus(fmt.Sprintf("Error: %v", err), false, false, nil, state.HysteresisStatus{}, state.HysteresisStatus{})
		return
	}

	domesticHotWaterTempCelsius, err := loadDomesticWaterTemperature(client, cfg.MyUplink.DeviceID)
	if err != nil {
		log.Printf("Failed to get domestic hot water temperature: %v", err)
	}

	localNow := time.Now().In(time.Local)
	switchOnHysteresis := cfg.HotWaterSwitchOnHysteresisDuration()
	powerThreshold := cfg.HotWaterPowerThreshold()

	// The use-water daemon keeps only its domain-specific rules here.
	// Polling, config loading, PV reads, myUplink client reuse, and hysteresis
	// formatting live in pkg/daemoncore.
	if extraHotWaterActive {
		handleActiveExtraHotWater(pvData.Power, pvData.Soc, powerThreshold, domesticHotWaterTempCelsius)
		return
	}

	timingState.ConditionsNotMetSince = time.Time{}
	activationAllowed := localNow.Hour() < cfg.HotWaterActivationCutoffHour()

	if !energyConditionsMet(pvData.Power, pvData.Soc, powerThreshold, cfg.ThresholdsHotWater.Soc) {
		// PV-Leistung oder Batteriestand reichen aktuell noch nicht fuer Extra-WW.
		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus("Bedingungen nicht erfuellt", false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Println("Conditions not met for extra hot water")
	} else if operationMode, err := client.GetOperationMode(cfg.MyUplink.DeviceID); err != nil {
		// Die Energiewerte passen, aber der Betriebszustand der WP konnte nicht gelesen werden.
		timingState.ConditionsMetSince = time.Time{}
		log.Printf("Failed to get heat pump operation mode: %v", err)
		state.SetHotWaterStatus(fmt.Sprintf("Fehler: %v", err), false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
	} else if operationMode.Value != myuplink.OperationModeOptions.HeatingOperation &&
		operationMode.Value != myuplink.OperationModeOptions.DomesticHotWater &&
		operationMode.Value != myuplink.OperationModeOptions.ForcedDefrosting {
		// Die WP laeuft in einer Betriebsart, in der Extra-WW bewusst nicht gestartet wird.
		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus(
			fmt.Sprintf("Bedingungen erfuellt, aber WP laeuft nicht (Betriebsart: %s)", operationMode.Text),
			false,
			false,
			domesticHotWaterTempCelsius,
			state.HysteresisStatus{},
			state.HysteresisStatus{},
		)
		log.Printf("Bedingungen erfuellt, aber WP laeuft nicht; Betriebsart ist %s", operationMode.Text)
	} else if !activationAllowed {
		// Nach der konfigurierten Tagesgrenze wird kein neuer Extra-WW-Zyklus mehr gestartet.
		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus("Zu spaet fuer Aktivierung", false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Println("Conditions met, but activation cutoff for today has passed")
	} else if timingState.ConditionsMetSince.IsZero() {
		// Die Einschalt-Hysterese startet, sobald alle Freigabebedingungen erstmals gleichzeitig gelten.
		timingState.ConditionsMetSince = localNow
		state.SetHotWaterStatus(
			fmt.Sprintf("Bedingungen erfuellt, Einschaltverzoegerung laeuft (%s)", switchOnHysteresis),
			false,
			false,
			domesticHotWaterTempCelsius,
			daemoncore.HysteresisStatus(switchOnHysteresis, timingState.ConditionsMetSince, localNow),
			state.HysteresisStatus{},
		)
		log.Printf("Conditions met, starting switch-on hysteresis at %s", localNow.Format("2006-01-02 15:04:05 MST"))
	} else if localNow.Sub(timingState.ConditionsMetSince) < switchOnHysteresis {
		// Die Bedingungen bleiben stabil, aber die Einschalt-Hysterese ist noch nicht abgelaufen.
		remaining := switchOnHysteresis - localNow.Sub(timingState.ConditionsMetSince)
		state.SetHotWaterStatus(
			fmt.Sprintf("Einschalten in %s", remaining.Round(time.Second)),
			false,
			false,
			domesticHotWaterTempCelsius,
			daemoncore.HysteresisStatus(switchOnHysteresis, timingState.ConditionsMetSince, localNow),
			state.HysteresisStatus{},
		)
		log.Printf("Switch-on hysteresis still active, %s remaining", remaining.Round(time.Second))
	} else {
		// Alle Bedingungen inklusive Hysterese sind erfuellt; Extra-WW wird jetzt aktiviert.
		extraTemperature := cfg.HotWaterExtraTemperature()
		if extraTemperature > 0 {
			err = client.SetExtraHotWaterTemperature(cfg.MyUplink.DeviceID, extraTemperature)
			if err != nil {
				log.Printf("Failed to set extra hot water temperature: %v", err)
				state.SetHotWaterStatus(fmt.Sprintf("Fehler: %v", err), false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
				return
			}
		}

		err = client.SetExtraHotWater(cfg.MyUplink.DeviceID, true)
		if err != nil {
			log.Printf("Failed to set extra hot water: %v", err)
			state.SetHotWaterStatus(fmt.Sprintf("Fehler: %v", err), false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
			return
		}

		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus("Extra-WW eingeschaltet", true, true, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Printf("Extra hot water activated at %s with PV power %.2f W", localNow.Format("2006-01-02 15:04:05 MST"), pvData.Power)
	}
}

func Disable() error {
	deps, err := daemoncore.LoadDependencies(config.DefaultPath, clientProvider)
	if err != nil {
		state.SetHotWaterStatus(fmt.Sprintf("Fehler: %v", err), false, false, nil, state.HysteresisStatus{}, state.HysteresisStatus{})
		return err
	}

	return disableWithClient(deps.Config, deps.Client, "WW-Daemon deaktiviert")
}

func disableWithClient(cfg *config.Config, client interface {
	GetExtraHotWater(deviceID string) (bool, error)
	SetExtraHotWater(deviceID string, enabled bool) error
	SetExtraHotWaterTemperature(deviceID string, value float64) error
	GetDomesticWaterTemperature(deviceID string) (float64, error)
}, message string) error {
	timingState = daemoncore.TimingState{}

	domesticHotWaterTempCelsius, err := loadDomesticWaterTemperature(client, cfg.MyUplink.DeviceID)
	if err != nil {
		log.Printf("Failed to get domestic hot water temperature while disabling daemon: %v", err)
	}

	extraHotWaterActive, readErr := client.GetExtraHotWater(cfg.MyUplink.DeviceID)
	if readErr != nil {
		log.Printf("Failed to get extra hot water status while disabling daemon: %v", readErr)
	}

	state.SetHotWaterStatus(message, false, extraHotWaterActive, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
	return nil
}

func loadDomesticWaterTemperature(client interface {
	GetDomesticWaterTemperature(deviceID string) (float64, error)
}, deviceID string) (*float64, error) {
	temperature, err := client.GetDomesticWaterTemperature(deviceID)
	if err != nil {
		return nil, err
	}

	return &temperature, nil
}

func handleActiveExtraHotWater(pvPower, soc, powerThreshold float64, domesticHotWaterTempCelsius *float64) {
	timingState.ConditionsMetSince = time.Time{}
	timingState.ConditionsNotMetSince = time.Time{}

	if keepExtraHotWaterActive(pvPower, powerThreshold) {
		state.SetHotWaterStatus("Extra-WW aktiv", true, true, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Printf(
			"Extra hot water remains active with PV power %.2f W and SOC %.2f%%",
			pvPower,
			soc,
		)
		return
	}

	state.SetHotWaterStatus(
		"Extra-WW aktiv, Abschaltung erfolgt nur durch Heizungssystem",
		true,
		true,
		domesticHotWaterTempCelsius,
		state.HysteresisStatus{},
		state.HysteresisStatus{},
	)
	log.Printf(
		"Extra hot water stays active although PV power dropped to %.2f W; daemon does not switch it off",
		pvPower,
	)
}

func energyConditionsMet(pvPower, soc, powerThreshold, socThreshold float64) bool {
	return pvPower > powerThreshold && soc > socThreshold
}

func keepExtraHotWaterActive(pvPower, powerThreshold float64) bool {
	return pvPower > powerThreshold
}
