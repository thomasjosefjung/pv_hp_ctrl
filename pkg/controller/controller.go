package controller

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/heatingdaemon"
	"pv_hp_ctrl/pkg/hotwaterdaemon"
	"pv_hp_ctrl/pkg/pv"
	"pv_hp_ctrl/pkg/state"
	"strconv"
	"sync"
	"time"
)

//go:embed templates/status.html
var templateFS embed.FS

var statusTemplate = template.Must(template.New("status.html").Funcs(template.FuncMap{
	"formatHysteresis":        formatHysteresis,
	"formatTemperature":       formatTemperature,
	"formatTemperatureOffset": formatTemperatureOffset,
}).ParseFS(templateFS, "templates/status.html"))

var (
	configPath               = config.DefaultPath
	daemonSettingsMu         sync.Mutex
	applyHotWaterDaemonState = func(enabled bool) error {
		if enabled {
			hotwaterdaemon.RunTask()
			return nil
		}

		return hotwaterdaemon.Disable()
	}
	applyHeatingDaemonState = func(enabled bool) error {
		if enabled {
			heatingdaemon.RunTask()
			return nil
		}

		return heatingdaemon.Disable()
	}
)

type daemonStatusResponse struct {
	LastCheck                  time.Time              `json:"lastCheck"`
	Message                    string                 `json:"message"`
	Enabled                    bool                   `json:"enabled"`
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
	LastCheck                  time.Time              `json:"lastCheck"`
	Message                    string                 `json:"message"`
	Enabled                    bool                   `json:"enabled"`
	IsActive                   bool                   `json:"isActive"`
	SwitchOnHysteresis         state.HysteresisStatus `json:"switchOnHysteresis"`
	SwitchOnHysteresisMinutes  int                    `json:"switchOnHysteresisMinutes"`
	ExtraHotWaterActive         bool     `json:"extraHotWaterActive"`
	DomesticHotWaterTempCelsius *float64 `json:"domesticHotWaterTempCelsius,omitempty"`
}

type heatingStatusResponse struct {
	daemonStatusResponse
	TemperatureOffset float64 `json:"temperatureOffset"`
	NormalOffset      float64 `json:"normalOffset"`
	PVOffset          float64 `json:"pvOffset"`
}

type hotWaterThresholdsResponse struct {
	Power                      float64 `json:"power"`
	Soc                        float64 `json:"soc"`
	SwitchOnHysteresisMinutes  int     `json:"switchOnHysteresisMinutes"`
	ActivationCutoff           int     `json:"activationCutoff"`
	ExtraHotWaterTemperature   float64 `json:"extraHotWaterTemperature"`
}

type heatingThresholdsResponse struct {
	Power                      float64 `json:"power"`
	Soc                        float64 `json:"soc"`
	SwitchOnHysteresisMinutes  int     `json:"switchOnHysteresisMinutes"`
	SwitchOffHysteresisMinutes int     `json:"switchOffHysteresisMinutes"`
	NormalOffset               float64 `json:"normalOffset"`
	PVOffset                   float64 `json:"pvOffset"`
}

type configStatusResponse struct {
	Path     string                     `json:"path"`
	HotWater hotWaterThresholdsResponse `json:"hotWater"`
	Heating  heatingThresholdsResponse  `json:"heating"`
}

type statusResponse struct {
	Energy   energyStatusResponse   `json:"energy"`
	HotWater hotWaterStatusResponse `json:"hotWater"`
	Heating  heatingStatusResponse  `json:"heating"`
	Config   configStatusResponse   `json:"config"`
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

func UpdateDaemons(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	hotWaterEnabled, err := strconv.ParseBool(r.FormValue("hotWaterEnabled"))
	if err != nil {
		http.Error(w, "Invalid hotWaterEnabled value", http.StatusBadRequest)
		return
	}

	heatingEnabled, err := strconv.ParseBool(r.FormValue("heatingEnabled"))
	if err != nil {
		http.Error(w, "Invalid heatingEnabled value", http.StatusBadRequest)
		return
	}

	daemonSettingsMu.Lock()
	defer daemonSettingsMu.Unlock()

	cfg, err := config.Load(configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	cfg.SetHotWaterDaemonEnabled(hotWaterEnabled)
	cfg.SetHeatingDaemonEnabled(heatingEnabled)
	if err := cfg.Save(configPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

	if err := applyHotWaterDaemonState(hotWaterEnabled); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update hot-water daemon: %v", err), http.StatusInternalServerError)
		return
	}

	if err := applyHeatingDaemonState(heatingEnabled); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update heating daemon: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(buildStatusResponse(state.GetStatus())); err != nil {
		http.Error(w, "Failed to encode status", http.StatusInternalServerError)
	}
}

func UpdateThresholds(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	hotWaterPower, err := parseFloatFormValue(r, "hotWaterPower")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hotWaterSoc, err := parseFloatFormValue(r, "hotWaterSoc")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hotWaterSwitchOn, err := parseIntFormValue(r, "hotWaterSwitchOnHysteresisMinutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hotWaterActivationCutoff, err := parseIntFormValue(r, "hotWaterActivationCutoff")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hotWaterExtraTemperature, err := parseFloatFormValue(r, "hotWaterExtraTemperature")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	heatingPower, err := parseFloatFormValue(r, "heatingPower")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	heatingSoc, err := parseFloatFormValue(r, "heatingSoc")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	heatingSwitchOn, err := parseIntFormValue(r, "heatingSwitchOnHysteresisMinutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	heatingSwitchOff, err := parseIntFormValue(r, "heatingSwitchOffHysteresisMinutes")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	heatingNormalOffset, err := parseFloatFormValue(r, "heatingNormalOffset")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	heatingPVOffset, err := parseFloatFormValue(r, "heatingPVOffset")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	daemonSettingsMu.Lock()
	defer daemonSettingsMu.Unlock()

	cfg, err := config.Load(configPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load config: %v", err), http.StatusInternalServerError)
		return
	}

	cfg.ThresholdsHotWater.Power = hotWaterPower
	cfg.ThresholdsHotWater.Soc = hotWaterSoc
	cfg.ThresholdsHotWater.SwitchOnHysteresisMinutes = hotWaterSwitchOn
	cfg.ThresholdsHotWater.ActivationCutoff = hotWaterActivationCutoff
	cfg.ThresholdsHotWater.ExtraHotWaterTemperature = hotWaterExtraTemperature
	cfg.ThresholdsHeating.Power = heatingPower
	cfg.ThresholdsHeating.Soc = heatingSoc
	cfg.ThresholdsHeating.SwitchOnHysteresisMinutes = heatingSwitchOn
	cfg.ThresholdsHeating.SwitchOffHysteresisMinutes = heatingSwitchOff
	cfg.ThresholdsHeating.NormalOffset = float64Ptr(heatingNormalOffset)
	cfg.ThresholdsHeating.PVOffset = float64Ptr(heatingPVOffset)

	if err := cfg.Save(configPath); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save config: %v", err), http.StatusInternalServerError)
		return
	}

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
			LastCheck:                  status.HotWater.LastCheck,
			Message:                    status.HotWater.Message,
			Enabled:                    cfg.HotWaterDaemonEnabled(),
			IsActive:                   status.HotWater.IsActive,
			SwitchOnHysteresis:         status.HotWater.SwitchOnHysteresis,
			SwitchOnHysteresisMinutes:  int(cfg.HotWaterSwitchOnHysteresisDuration() / time.Minute),
			ExtraHotWaterActive:         status.HotWater.ExtraHotWaterActive,
			DomesticHotWaterTempCelsius: status.HotWater.DomesticHotWaterTempCelsius,
		},
		Heating: heatingStatusResponse{
			daemonStatusResponse: daemonStatusResponse{
				LastCheck:                  status.Heating.LastCheck,
				Message:                    status.Heating.Message,
				Enabled:                    cfg.HeatingDaemonEnabled(),
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
		Config: configStatusResponse{
			Path: resolvedConfigPath(),
			HotWater: hotWaterThresholdsResponse{
				Power:                      cfg.HotWaterPowerThreshold(),
				Soc:                        cfg.HotWaterSocThreshold(),
				SwitchOnHysteresisMinutes:  int(cfg.HotWaterSwitchOnHysteresisDuration() / time.Minute),
				ActivationCutoff:           cfg.HotWaterActivationCutoffHour(),
				ExtraHotWaterTemperature:   cfg.HotWaterExtraTemperature(),
			},
			Heating: heatingThresholdsResponse{
				Power:                      cfg.HeatingPowerThreshold(),
				Soc:                        cfg.HeatingSocThreshold(),
				SwitchOnHysteresisMinutes:  int(cfg.HeatingSwitchOnHysteresisDuration() / time.Minute),
				SwitchOffHysteresisMinutes: int(cfg.HeatingSwitchOffHysteresisDuration() / time.Minute),
				NormalOffset:               cfg.HeatingNormalOffset(),
				PVOffset:                   cfg.HeatingPVOffset(),
			},
		},
	}
}

func loadConfigWithDefaults() *config.Config {
	cfg, err := config.Load(configPath)
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

func parseFloatFormValue(r *http.Request, key string) (float64, error) {
	value := r.FormValue(key)
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value", key)
	}

	return parsed, nil
}

func parseIntFormValue(r *http.Request, key string) (int, error) {
	value := r.FormValue(key)
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s value", key)
	}

	return parsed, nil
}

func float64Ptr(value float64) *float64 {
	return &value
}

func resolvedConfigPath() string {
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return configPath
	}

	return absPath
}
