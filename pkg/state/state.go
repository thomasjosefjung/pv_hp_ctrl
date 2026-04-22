package state

import (
	"pv_hp_ctrl/pkg/pv"
	"sync"
	"time"
)

// Status holds the current application state.
type Status struct {
	Energy   EnergyStatus   `json:"energy"`
	HotWater HotWaterStatus `json:"hotWater"`
	Heating  HeatingStatus  `json:"heating"`
}

type EnergyStatus struct {
	PVData *pv.PVData `json:"pvData,omitempty"`
}

type BaseDaemonStatus struct {
	LastCheck           time.Time        `json:"lastCheck"`
	Message             string           `json:"message"`
	IsActive            bool             `json:"isActive"`
	SwitchOnHysteresis  HysteresisStatus `json:"switchOnHysteresis"`
	SwitchOffHysteresis HysteresisStatus `json:"switchOffHysteresis"`
}

type HotWaterStatus struct {
	BaseDaemonStatus
	ExtraHotWaterActive        bool     `json:"extraHotWaterActive"`
	DomesticHotWaterTempCelsius *float64 `json:"domesticHotWaterTempCelsius,omitempty"`
}

type HeatingStatus struct {
	BaseDaemonStatus
	TemperatureOffset float64 `json:"temperatureOffset"`
}

type HysteresisStatus struct {
	Active           bool `json:"active"`
	RemainingSeconds int  `json:"remainingSeconds"`
}

var (
	currentStatus Status
	subscribers   = make(map[chan Status]struct{})
	mutex         sync.RWMutex
)

// GetStatus returns a copy of the current status.
func GetStatus() Status {
	mutex.RLock()
	defer mutex.RUnlock()
	return currentStatus
}

func Subscribe() (<-chan Status, func()) {
	updates := make(chan Status, 1)

	mutex.Lock()
	subscribers[updates] = struct{}{}
	snapshot := currentStatus
	mutex.Unlock()

	deliverStatus(updates, snapshot)

	return updates, func() {
		mutex.Lock()
		delete(subscribers, updates)
		mutex.Unlock()
	}
}

// SetStatus updates the current status.

func SetEnergyStatus(pvData *pv.PVData) {
	mutex.Lock()
	currentStatus.Energy.PVData = pvData

	snapshot := currentStatus
	listeners := make([]chan Status, 0, len(subscribers))
	for subscriber := range subscribers {
		listeners = append(listeners, subscriber)
	}
	mutex.Unlock()

	for _, subscriber := range listeners {
		deliverStatus(subscriber, snapshot)
	}
}

func SetHotWaterStatus(message string, isActive, extraHotWaterActive bool, domesticHotWaterTempCelsius *float64, switchOnHysteresis, switchOffHysteresis HysteresisStatus) {
	mutex.Lock()
	currentStatus.HotWater.LastCheck = time.Now()
	currentStatus.HotWater.Message = message
	currentStatus.HotWater.IsActive = isActive
	currentStatus.HotWater.ExtraHotWaterActive = extraHotWaterActive
	currentStatus.HotWater.DomesticHotWaterTempCelsius = domesticHotWaterTempCelsius
	currentStatus.HotWater.SwitchOnHysteresis = switchOnHysteresis
	currentStatus.HotWater.SwitchOffHysteresis = switchOffHysteresis

	snapshot := currentStatus
	listeners := make([]chan Status, 0, len(subscribers))
	for subscriber := range subscribers {
		listeners = append(listeners, subscriber)
	}
	mutex.Unlock()

	for _, subscriber := range listeners {
		deliverStatus(subscriber, snapshot)
	}
}

func SetHeatingStatus(message string, isActive bool, temperatureOffset float64, switchOnHysteresis, switchOffHysteresis HysteresisStatus) {
	mutex.Lock()
	currentStatus.Heating.LastCheck = time.Now()
	currentStatus.Heating.Message = message
	currentStatus.Heating.IsActive = isActive
	currentStatus.Heating.TemperatureOffset = temperatureOffset
	currentStatus.Heating.SwitchOnHysteresis = switchOnHysteresis
	currentStatus.Heating.SwitchOffHysteresis = switchOffHysteresis

	snapshot := currentStatus
	listeners := make([]chan Status, 0, len(subscribers))
	for subscriber := range subscribers {
		listeners = append(listeners, subscriber)
	}
	mutex.Unlock()

	for _, subscriber := range listeners {
		deliverStatus(subscriber, snapshot)
	}
}

func deliverStatus(ch chan Status, status Status) {
	select {
	case ch <- status:
	default:
		select {
		case <-ch:
		default:
		}

		select {
		case ch <- status:
		default:
		}
	}
}
