package state

import (
	"pv_hp_ctrl/pkg/pv"
	"sync"
	"time"
)

// Status holds the current application state.
type Status struct {
	LastCheck time.Time
	Message   string
	PVData    *pv.PVData
	IsActive  bool
}

var (
	currentStatus Status
	mutex         sync.RWMutex
)

// GetStatus returns a copy of the current status.
func GetStatus() Status {
	mutex.RLock()
	defer mutex.RUnlock()
	return currentStatus
}

// SetStatus updates the current status.
func SetStatus(message string, pvData *pv.PVData, isActive bool) {
	mutex.Lock()
	defer mutex.Unlock()
	currentStatus.LastCheck = time.Now()
	currentStatus.Message = message
	currentStatus.PVData = pvData
	currentStatus.IsActive = isActive
}
