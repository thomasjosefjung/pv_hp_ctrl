package controller

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/pv"
	"pv_hp_ctrl/pkg/state"
	"time"
)

//go:embed templates/status.html
var templateFS embed.FS

var statusTemplate = template.Must(template.New("status.html").Funcs(template.FuncMap{
	"formatHysteresis":        formatHysteresis,
	"formatTemperature":       formatTemperature,
	"formatTemperatureOffset": formatTemperatureOffset,
}).ParseFS(templateFS, "templates/status.html"))

type daemonStatusResponse struct {
	LastCheck                  time.Time              `json:"lastCheck"`
	Message                    string                 `json:"message"`
	IsActive                   bool                   `json:"isActive"`
	SwitchOnHysteresis         state.HysteresisStatus `json:"switchOnHysteresis"`
	SwitchOffHysteresis        state.HysteresisStatus `json:"switchOffHysteresis"`
	SwitchOnHysteresisMinutes  int                    `json:"switchOnHysteresisMinutes"`
	SwitchOffHysteresisMinutes int                    `json:"switchOffHysteresisMinutes"`
}

type energyStatusResponse struct {
	PVData *pv.PVData `json:"pvData,omitempty"`
}

type hotWaterStatusResponse struct {
	daemonStatusResponse
	ExtraHotWaterActive         bool     `json:"extraHotWaterActive"`
	DomesticHotWaterTempCelsius *float64 `json:"domesticHotWaterTempCelsius,omitempty"`
}

type heatingStatusResponse struct {
	daemonStatusResponse
	TemperatureOffset float64 `json:"temperatureOffset"`
	NormalOffset      float64 `json:"normalOffset"`
	PVOffset          float64 `json:"pvOffset"`
}

type statusResponse struct {
	Energy   energyStatusResponse   `json:"energy"`
	HotWater hotWaterStatusResponse `json:"hotWater"`
	Heating  heatingStatusResponse  `json:"heating"`
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK")
}

func StatusPage(w http.ResponseWriter, r *http.Request) {
	if err := statusTemplate.Execute(w, buildStatusResponse(state.GetStatus())); err != nil {
		http.Error(w, "Failed to execute template", http.StatusInternalServerError)
		return
	}
}

func StatusAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildStatusResponse(state.GetStatus())); err != nil {
		http.Error(w, "Failed to encode status", http.StatusInternalServerError)
	}
}

func StatusStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	updates, unsubscribe := state.Subscribe()
	defer unsubscribe()

	keepAliveTicker := time.NewTicker(25 * time.Second)
	defer keepAliveTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case status := <-updates:
			payload, err := json.Marshal(buildStatusResponse(status))
			if err != nil {
				continue
			}

			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		case <-keepAliveTicker.C:
			if _, err := fmt.Fprint(w, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func buildStatusResponse(status state.Status) statusResponse {
	cfg := loadConfigWithDefaults()

	return statusResponse{
		Energy: energyStatusResponse{
			PVData: status.Energy.PVData,
		},
		HotWater: hotWaterStatusResponse{
			daemonStatusResponse: daemonStatusResponse{
				LastCheck:                  status.HotWater.LastCheck,
				Message:                    status.HotWater.Message,
				IsActive:                   status.HotWater.IsActive,
				SwitchOnHysteresis:         status.HotWater.SwitchOnHysteresis,
				SwitchOffHysteresis:        status.HotWater.SwitchOffHysteresis,
				SwitchOnHysteresisMinutes:  int(cfg.HotWaterSwitchOnHysteresisDuration() / time.Minute),
				SwitchOffHysteresisMinutes: int(cfg.HotWaterSwitchOffHysteresisDuration() / time.Minute),
			},
			ExtraHotWaterActive:         status.HotWater.ExtraHotWaterActive,
			DomesticHotWaterTempCelsius: status.HotWater.DomesticHotWaterTempCelsius,
		},
		Heating: heatingStatusResponse{
			daemonStatusResponse: daemonStatusResponse{
				LastCheck:                  status.Heating.LastCheck,
				Message:                    status.Heating.Message,
				IsActive:                   status.Heating.IsActive,
				SwitchOnHysteresis:         status.Heating.SwitchOnHysteresis,
				SwitchOffHysteresis:        status.Heating.SwitchOffHysteresis,
				SwitchOnHysteresisMinutes:  int(cfg.HeatingSwitchOnHysteresisDuration() / time.Minute),
				SwitchOffHysteresisMinutes: int(cfg.HeatingSwitchOffHysteresisDuration() / time.Minute),
			},
			TemperatureOffset: status.Heating.TemperatureOffset,
			NormalOffset:      cfg.HeatingNormalOffset(),
			PVOffset:          cfg.HeatingPVOffset(),
		},
	}
}

func loadConfigWithDefaults() *config.Config {
	cfg, err := config.Load(config.DefaultPath)
	if err != nil {
		return &config.Config{}
	}

	return cfg
}

func formatHysteresis(active bool, remainingSeconds, totalMinutes int) string {
	if totalMinutes <= 0 {
		return "--"
	}

	if !active {
		return fmt.Sprintf("-- / %d min", totalMinutes)
	}

	return fmt.Sprintf("%s / %d min", formatRemainingSeconds(remainingSeconds), totalMinutes)
}

func formatRemainingSeconds(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}

	duration := time.Duration(seconds) * time.Second
	hours := int(duration / time.Hour)
	minutes := int((duration % time.Hour) / time.Minute)
	secs := int((duration % time.Minute) / time.Second)

	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}

	return fmt.Sprintf("%d:%02d", minutes, secs)
}

func formatTemperatureOffset(value float64) string {
	return fmt.Sprintf("%+.1f C", value)
}

func formatTemperature(value *float64) string {
	if value == nil {
		return "--"
	}

	return fmt.Sprintf("%.1f C", *value)
}
