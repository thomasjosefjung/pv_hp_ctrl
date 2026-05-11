package heatingdaemon

import (
	"fmt"
	"log"
	"math"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/daemoncore"
	"pv_hp_ctrl/pkg/myuplink"
	"pv_hp_ctrl/pkg/state"
	"time"
)

var (
	clientProvider = &daemoncore.ClientProvider{}
	// timingState keeps the hysteresis windows stable across polling cycles.
	//
	// The heating daemon has different domain rules than the hot-water daemon,
	// but it composes the same shared timing helper instead of inheriting fields.
	timingState = daemoncore.TimingState{}
)

func RunTask() {
	deps, err := daemoncore.LoadDependencies(config.DefaultPath, clientProvider)
	if err != nil {
		log.Printf("Failed to load daemon dependencies: %v", err)
		state.SetHeatingStatus(fmt.Sprintf("Error: %v", err), false, 0, state.HysteresisStatus{}, state.HysteresisStatus{})
		return
	}

	cfg := deps.Config
	client := deps.Client
	if !cfg.HeatingDaemonEnabled() {
		if err := disableWithClient(cfg, client, "Heiz-Daemon deaktiviert"); err != nil {
			log.Printf("Failed to restore normal heating offset while daemon disabled: %v", err)
		}
		return
	}

	pvData := state.GetStatus().Energy.PVData
	if pvData == nil {
		state.SetHeatingStatus("Energiedaten fehlen", false, 0, state.HysteresisStatus{}, state.HysteresisStatus{})
		return
	}

	currentOffset, err := client.GetHeatingTemperatureOffset(cfg.MyUplink.DeviceID)
	if err != nil {
		log.Printf("Failed to get heating temperature offset: %v", err)
		state.SetHeatingStatus(fmt.Sprintf("Error: %v", err), false, 0, state.HysteresisStatus{}, state.HysteresisStatus{})
		return
	}

	localNow := time.Now().In(time.Local)
	switchOnHysteresis := cfg.HeatingSwitchOnHysteresisDuration()
	switchOffHysteresis := cfg.HeatingSwitchOffHysteresisDuration()
	powerThreshold := cfg.HeatingPowerThreshold()
	socThreshold := cfg.HeatingSocThreshold()
	normalOffset := cfg.HeatingNormalOffset()
	pvOffset := cfg.HeatingPVOffset()
	offsetActive := offsetsEqual(currentOffset, pvOffset)

	// The shared runner handles the repetitive technical plumbing.
	// This file only contains the business decision for parameter 5001.
	if offsetActive {
		timingState.ConditionsMetSince = time.Time{}

		if heatingConditionsMet(pvData.Power, pvData.Soc, powerThreshold, socThreshold) {
			// Die PV-Bedingungen passen weiter; der aktive Heiz-Offset bleibt unveraendert eingeschaltet.
			timingState.ConditionsNotMetSince = time.Time{}
			state.SetHeatingStatus("Heiz-Offset aktiv", true, currentOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
			log.Printf("Heating offset remains active with PV power %.2f W and offset %.1f C", pvData.Power, currentOffset)
		} else {
			if timingState.ConditionsNotMetSince.IsZero() {
				// Die Abschalt-Hysterese startet, sobald die Freigabebedingungen erstmals nicht mehr gelten.
				timingState.ConditionsNotMetSince = localNow
				state.SetHeatingStatus(
					fmt.Sprintf("Heiz-Offset aktiv, Ausschaltverzoegerung laeuft (%s)", switchOffHysteresis),
					true,
					currentOffset,
					state.HysteresisStatus{},
					daemoncore.HysteresisStatus(switchOffHysteresis, timingState.ConditionsNotMetSince, localNow),
				)
				log.Printf("Heating offset no longer meets conditions, starting switch-off hysteresis at %s", localNow.Format("2006-01-02 15:04:05 MST"))
			} else if localNow.Sub(timingState.ConditionsNotMetSince) < switchOffHysteresis {
				// Die Bedingungen bleiben zu schlecht, aber die Ausschalt-Hysterese laeuft noch.
				remaining := switchOffHysteresis - localNow.Sub(timingState.ConditionsNotMetSince)
				state.SetHeatingStatus(
					fmt.Sprintf("Heiz-Offset aktiv, Ausschalten in %s", remaining.Round(time.Second)),
					true,
					currentOffset,
					state.HysteresisStatus{},
					daemoncore.HysteresisStatus(switchOffHysteresis, timingState.ConditionsNotMetSince, localNow),
				)
				log.Printf("Heating offset still active during switch-off hysteresis, %s remaining", remaining.Round(time.Second))
			} else {
				// Die Bedingungen sind lang genug unterschritten; der Heiz-Offset wird auf Normalwert zurueckgesetzt.
				err = client.SetHeatingTemperatureOffset(cfg.MyUplink.DeviceID, normalOffset)
				if err != nil {
					log.Printf("Failed to reset heating temperature offset: %v", err)
					state.SetHeatingStatus(fmt.Sprintf("Fehler: %v", err), true, currentOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
					return
				}

				timingState.ConditionsNotMetSince = time.Time{}
				state.SetHeatingStatus("Heiz-Offset ausgeschaltet", false, normalOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
				log.Printf("Heating offset deactivated at %s with PV power %.2f W", localNow.Format("2006-01-02 15:04:05 MST"), pvData.Power)
			}
		}
	} else {
		timingState.ConditionsNotMetSince = time.Time{}

		if !heatingConditionsMet(pvData.Power, pvData.Soc, powerThreshold, socThreshold) {
			// PV-Leistung oder Batteriestand reichen aktuell noch nicht fuer den Heiz-Offset.
			timingState.ConditionsMetSince = time.Time{}
			state.SetHeatingStatus("Bedingungen nicht erfuellt", false, currentOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
			log.Println("Conditions not met for heating offset")
		} else if operationMode, err := client.GetOperationMode(cfg.MyUplink.DeviceID); err != nil {
			// Die Energiewerte passen, aber der Betriebszustand der WP konnte nicht gelesen werden.
			timingState.ConditionsMetSince = time.Time{}
			log.Printf("Failed to get heat pump operation mode: %v", err)
			state.SetHeatingStatus(fmt.Sprintf("Fehler: %v", err), false, currentOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
		} else if !operationModeAllowsHeatingOffset(operationMode) {
			// Die WP laeuft in einer Betriebsart, in der der Heiz-Offset bewusst nicht aktiviert wird.
			timingState.ConditionsMetSince = time.Time{}
			state.SetHeatingStatus(
				fmt.Sprintf("Bedingungen erfuellt, aber WP laeuft nicht (Betriebsart: %s)", operationMode.Text),
				false,
				currentOffset,
				state.HysteresisStatus{},
				state.HysteresisStatus{},
			)
			log.Printf("Bedingungen erfuellt, aber WP laeuft nicht; Betriebsart ist %s", operationMode.Text)
		} else if timingState.ConditionsMetSince.IsZero() {
			// Die Einschalt-Hysterese startet, sobald alle Freigabebedingungen erstmals gleichzeitig gelten.
			timingState.ConditionsMetSince = localNow
			state.SetHeatingStatus(
				fmt.Sprintf("Bedingungen erfuellt, Einschaltverzoegerung laeuft (%s)", switchOnHysteresis),
				false,
				currentOffset,
				daemoncore.HysteresisStatus(switchOnHysteresis, timingState.ConditionsMetSince, localNow),
				state.HysteresisStatus{},
			)
			log.Printf("Conditions met for heating offset, starting switch-on hysteresis at %s", localNow.Format("2006-01-02 15:04:05 MST"))
		} else if localNow.Sub(timingState.ConditionsMetSince) < switchOnHysteresis {
			// Die Bedingungen bleiben stabil, aber die Einschalt-Hysterese ist noch nicht abgelaufen.
			remaining := switchOnHysteresis - localNow.Sub(timingState.ConditionsMetSince)
			state.SetHeatingStatus(
				fmt.Sprintf("Einschalten in %s", remaining.Round(time.Second)),
				false,
				currentOffset,
				daemoncore.HysteresisStatus(switchOnHysteresis, timingState.ConditionsMetSince, localNow),
				state.HysteresisStatus{},
			)
			log.Printf("Heating switch-on hysteresis still active, %s remaining", remaining.Round(time.Second))
		} else {
			// Alle Bedingungen inklusive Hysterese sind erfuellt; der PV-Heiz-Offset wird jetzt gesetzt.
			err = client.SetHeatingTemperatureOffset(cfg.MyUplink.DeviceID, pvOffset)
			if err != nil {
				log.Printf("Failed to set heating temperature offset: %v", err)
				state.SetHeatingStatus(fmt.Sprintf("Fehler: %v", err), false, currentOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
				return
			}

			timingState.ConditionsMetSince = time.Time{}
			state.SetHeatingStatus("Heiz-Offset eingeschaltet", true, pvOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
			log.Printf("Heating offset activated at %s with PV power %.2f W", localNow.Format("2006-01-02 15:04:05 MST"), pvData.Power)
		}
	}
}

func Disable() error {
	deps, err := daemoncore.LoadDependencies(config.DefaultPath, clientProvider)
	if err != nil {
		state.SetHeatingStatus(fmt.Sprintf("Fehler: %v", err), false, 0, state.HysteresisStatus{}, state.HysteresisStatus{})
		return err
	}

	return disableWithClient(deps.Config, deps.Client, "Heiz-Daemon deaktiviert")
}

func disableWithClient(cfg *config.Config, client interface {
	GetHeatingTemperatureOffset(deviceID string) (float64, error)
	SetHeatingTemperatureOffset(deviceID string, value float64) error
}, message string) error {
	timingState = daemoncore.TimingState{}

	currentOffset, readErr := client.GetHeatingTemperatureOffset(cfg.MyUplink.DeviceID)
	if readErr != nil {
		log.Printf("Failed to get heating temperature offset while disabling daemon: %v", readErr)
		currentOffset = cfg.HeatingNormalOffset()
	}

	normalOffset := cfg.HeatingNormalOffset()
	if err := client.SetHeatingTemperatureOffset(cfg.MyUplink.DeviceID, normalOffset); err != nil {
		state.SetHeatingStatus(
			fmt.Sprintf("Heiz-Daemon deaktiviert, Ruecksetzen fehlgeschlagen: %v", err),
			false,
			currentOffset,
			state.HysteresisStatus{},
			state.HysteresisStatus{},
		)
		return err
	}

	state.SetHeatingStatus(message, false, normalOffset, state.HysteresisStatus{}, state.HysteresisStatus{})
	return nil
}

func heatingConditionsMet(pvPower, soc, powerThreshold, socThreshold float64) bool {
	return pvPower > powerThreshold && soc > socThreshold
}

func operationModeAllowsHeatingOffset(operationMode myuplink.OperationMode) bool {
	return operationMode.Value == myuplink.OperationModeOptions.HeatingOperation ||
		operationMode.Value == myuplink.OperationModeOptions.DomesticHotWater ||
		operationMode.Value == myuplink.OperationModeOptions.ForcedDefrosting
}

func offsetsEqual(left, right float64) bool {
	return math.Abs(left-right) < 0.0001
}
