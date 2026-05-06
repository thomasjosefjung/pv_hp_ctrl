package hotwaterdaemon

import (
	"fmt"
	"log"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/daemoncore"
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
		if err := disableWithClient(cfg, client, "Hot-water daemon disabled; extra hot water forced off"); err != nil {
			log.Printf("Failed to force extra hot water off while daemon disabled: %v", err)
		}
		return
	}

	pvData := state.GetStatus().Energy.PVData
	if pvData == nil {
		state.SetHotWaterStatus("Shared energy status unavailable", false, false, nil, state.HysteresisStatus{}, state.HysteresisStatus{})
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
	switchOffHysteresis := cfg.HotWaterSwitchOffHysteresisDuration()
	powerThreshold := cfg.HotWaterPowerThreshold()

	// The use-water daemon keeps only its domain-specific rules here.
	// Polling, config loading, PV reads, myUplink client reuse, and hysteresis
	// formatting live in pkg/daemoncore.
	if extraHotWaterActive {
		timingState.ConditionsMetSince = time.Time{}

		if keepExtraHotWaterActive(pvData.Power, powerThreshold) {
			timingState.ConditionsNotMetSince = time.Time{}
			state.SetHotWaterStatus("Extra hot water active; PV power still above switch-off threshold", true, true, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
			log.Printf(
				"Extra hot water remains active with PV power %.2f W and SOC %.2f%%",
				pvData.Power,
				pvData.Soc,
			)
		} else {
			if timingState.ConditionsNotMetSince.IsZero() {
				timingState.ConditionsNotMetSince = localNow
				state.SetHotWaterStatus(
					fmt.Sprintf("Extra hot water active; PV power below switch-off threshold, hysteresis started (%s)", switchOffHysteresis),
					true,
					true,
					domesticHotWaterTempCelsius,
					state.HysteresisStatus{},
					daemoncore.HysteresisStatus(switchOffHysteresis, timingState.ConditionsNotMetSince, localNow),
				)
				log.Printf("PV power dropped below switch-off threshold, starting switch-off hysteresis at %s", localNow.Format("2006-01-02 15:04:05 MST"))
			} else if localNow.Sub(timingState.ConditionsNotMetSince) < switchOffHysteresis {
				remaining := switchOffHysteresis - localNow.Sub(timingState.ConditionsNotMetSince)
				state.SetHotWaterStatus(
					fmt.Sprintf("Extra hot water active; PV power below switch-off threshold, waiting %s before switch-off", remaining.Round(time.Second)),
					true,
					true,
					domesticHotWaterTempCelsius,
					state.HysteresisStatus{},
					daemoncore.HysteresisStatus(switchOffHysteresis, timingState.ConditionsNotMetSince, localNow),
				)
				log.Printf("Extra hot water still active during low-PV switch-off hysteresis, %s remaining", remaining.Round(time.Second))
			} else {
				err = client.SetExtraHotWater(cfg.MyUplink.DeviceID, false)
				if err != nil {
					log.Printf("Failed to disable extra hot water: %v", err)
					state.SetHotWaterStatus(fmt.Sprintf("Error: %v", err), true, true, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
					return
				}

				timingState.ConditionsNotMetSince = time.Time{}
				state.SetHotWaterStatus("Extra hot water deactivated; PV power stayed below switch-off threshold", false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
				log.Printf("Extra hot water deactivated at %s with PV power %.2f W", localNow.Format("2006-01-02 15:04:05 MST"), pvData.Power)
			}
		}
		return
	}

	timingState.ConditionsNotMetSince = time.Time{}
	activationAllowed := localNow.Hour() < cfg.HotWaterActivationCutoffHour()

	if !energyConditionsMet(pvData.Power, pvData.Soc, powerThreshold, cfg.ThresholdsHotWater.Soc) {
		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus("Conditions not met for extra hot water", false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Println("Conditions not met for extra hot water")
	} else if !activationAllowed {
		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus("Conditions met, but activation cutoff for today has passed", false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Println("Conditions met, but activation cutoff for today has passed")
	} else if timingState.ConditionsMetSince.IsZero() {
		timingState.ConditionsMetSince = localNow
		state.SetHotWaterStatus(
			fmt.Sprintf("Conditions met; switch-on hysteresis started (%s)", switchOnHysteresis),
			false,
			false,
			domesticHotWaterTempCelsius,
			daemoncore.HysteresisStatus(switchOnHysteresis, timingState.ConditionsMetSince, localNow),
			state.HysteresisStatus{},
		)
		log.Printf("Conditions met, starting switch-on hysteresis at %s", localNow.Format("2006-01-02 15:04:05 MST"))
	} else if localNow.Sub(timingState.ConditionsMetSince) < switchOnHysteresis {
		remaining := switchOnHysteresis - localNow.Sub(timingState.ConditionsMetSince)
		state.SetHotWaterStatus(
			fmt.Sprintf("Conditions met; waiting %s before switch-on", remaining.Round(time.Second)),
			false,
			false,
			domesticHotWaterTempCelsius,
			daemoncore.HysteresisStatus(switchOnHysteresis, timingState.ConditionsMetSince, localNow),
			state.HysteresisStatus{},
		)
		log.Printf("Switch-on hysteresis still active, %s remaining", remaining.Round(time.Second))
	} else {
		err = client.SetExtraHotWater(cfg.MyUplink.DeviceID, true)
		if err != nil {
			log.Printf("Failed to set extra hot water: %v", err)
			state.SetHotWaterStatus(fmt.Sprintf("Error: %v", err), false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
			return
		}

		timingState.ConditionsMetSince = time.Time{}
		state.SetHotWaterStatus("Extra hot water activated", true, true, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
		log.Printf("Extra hot water activated at %s with PV power %.2f W", localNow.Format("2006-01-02 15:04:05 MST"), pvData.Power)
	}
}

func Disable() error {
	deps, err := daemoncore.LoadDependencies(config.DefaultPath, clientProvider)
	if err != nil {
		state.SetHotWaterStatus(fmt.Sprintf("Error: %v", err), false, false, nil, state.HysteresisStatus{}, state.HysteresisStatus{})
		return err
	}

	return disableWithClient(deps.Config, deps.Client, "Hot-water daemon disabled; extra hot water forced off")
}

func disableWithClient(cfg *config.Config, client interface {
	GetExtraHotWater(deviceID string) (bool, error)
	SetExtraHotWater(deviceID string, enabled bool) error
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

	if err := client.SetExtraHotWater(cfg.MyUplink.DeviceID, false); err != nil {
		state.SetHotWaterStatus(
			fmt.Sprintf("Hot-water daemon disabled, but forcing extra hot water off failed: %v", err),
			false,
			extraHotWaterActive,
			domesticHotWaterTempCelsius,
			state.HysteresisStatus{},
			state.HysteresisStatus{},
		)
		return err
	}

	state.SetHotWaterStatus(message, false, false, domesticHotWaterTempCelsius, state.HysteresisStatus{}, state.HysteresisStatus{})
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

func energyConditionsMet(pvPower, soc, powerThreshold, socThreshold float64) bool {
	return pvPower > powerThreshold && soc > socThreshold
}

func keepExtraHotWaterActive(pvPower, powerThreshold float64) bool {
	return pvPower > powerThreshold
}
