package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"pv_hp_ctrl/config"
	"pv_hp_ctrl/pkg/pv"
	"pv_hp_ctrl/pkg/state"
)

type streamingResponseWriter struct {
	header      http.Header
	body        bytes.Buffer
	failOnWrite bool
}

func (w *streamingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}

	return w.header
}

func (w *streamingResponseWriter) Write(data []byte) (int, error) {
	_, _ = w.body.Write(data)
	if w.failOnWrite {
		return len(data), errors.New("stop after first event")
	}

	return len(data), nil
}

func (w *streamingResponseWriter) WriteHeader(statusCode int) {}

func (w *streamingResponseWriter) Flush() {}

func TestBuildStatusResponseUsesConfiguredValues(t *testing.T) {
	temp := 52.5
	pvData := &pv.PVData{Power: 4200.5, Soc: 87.3}
	status := state.Status{
		Energy: state.EnergyStatus{
			PVData: pvData,
		},
		HotWater: state.HotWaterStatus{
			BaseDaemonStatus: state.BaseDaemonStatus{
				LastCheck: time.Unix(100, 0),
				Message:   "ok",
				IsActive:  true,
				SwitchOnHysteresis: state.HysteresisStatus{
					Active:           true,
					RemainingSeconds: 120,
				},
			},
			ExtraHotWaterActive:         true,
			DomesticHotWaterTempCelsius: &temp,
		},
	}

	response := buildStatusResponse(status)

	if response.Energy.PVData != pvData {
		t.Fatalf("Energy.PVData = %v, want %v", response.Energy.PVData, pvData)
	}

	if response.HotWater.IsActive != status.HotWater.IsActive {
		t.Fatalf("HotWater.IsActive = %v, want %v", response.HotWater.IsActive, status.HotWater.IsActive)
	}

	if !response.HotWater.Enabled {
		t.Fatal("expected hot-water daemon to be enabled by default")
	}

	if response.HotWater.Message != status.HotWater.Message {
		t.Fatalf("HotWater.Message = %q, want %q", response.HotWater.Message, status.HotWater.Message)
	}

	if response.HotWater.SwitchOnHysteresisMinutes != 5 {
		t.Fatalf("HotWater.SwitchOnHysteresisMinutes = %d, want 5", response.HotWater.SwitchOnHysteresisMinutes)
	}

	if response.HotWater.SwitchOffHysteresisMinutes != 10 {
		t.Fatalf("HotWater.SwitchOffHysteresisMinutes = %d, want 10", response.HotWater.SwitchOffHysteresisMinutes)
	}

	if !response.HotWater.SwitchOnHysteresis.Active {
		t.Fatal("expected switch-on hysteresis state to be preserved")
	}

	if response.HotWater.SwitchOnHysteresis.RemainingSeconds != 120 {
		t.Fatalf("HotWater.SwitchOnHysteresis.RemainingSeconds = %d, want 120", response.HotWater.SwitchOnHysteresis.RemainingSeconds)
	}

	if response.HotWater.DomesticHotWaterTempCelsius == nil || *response.HotWater.DomesticHotWaterTempCelsius != temp {
		t.Fatalf("HotWater.DomesticHotWaterTempCelsius = %v, want %v", response.HotWater.DomesticHotWaterTempCelsius, temp)
	}

	if response.Heating.SwitchOnHysteresisMinutes != 5 {
		t.Fatalf("Heating.SwitchOnHysteresisMinutes = %d, want 5", response.Heating.SwitchOnHysteresisMinutes)
	}

	if !response.Heating.Enabled {
		t.Fatal("expected heating daemon to be enabled by default")
	}

	if response.Heating.SwitchOffHysteresisMinutes != 10 {
		t.Fatalf("Heating.SwitchOffHysteresisMinutes = %d, want 10", response.Heating.SwitchOffHysteresisMinutes)
	}

	if response.Heating.NormalOffset != -1.0 {
		t.Fatalf("Heating.NormalOffset = %v, want -1.0", response.Heating.NormalOffset)
	}

	if response.Heating.PVOffset != 1.0 {
		t.Fatalf("Heating.PVOffset = %v, want 1.0", response.Heating.PVOffset)
	}

	if response.Config.Path == "" {
		t.Fatal("expected config path to be present")
	}

	if response.Config.HotWater.Power != 5000 {
		t.Fatalf("Config.HotWater.Power = %v, want 5000", response.Config.HotWater.Power)
	}

	if response.Config.HotWater.ExtraHotWaterTemperature != 0 {
		t.Fatalf("Config.HotWater.ExtraHotWaterTemperature = %v, want 0", response.Config.HotWater.ExtraHotWaterTemperature)
	}

	if response.Config.Heating.SwitchOffHysteresisMinutes != 10 {
		t.Fatalf("Config.Heating.SwitchOffHysteresisMinutes = %d, want 10", response.Config.Heating.SwitchOffHysteresisMinutes)
	}
}

func TestFormatHysteresis(t *testing.T) {
	if got := formatHysteresis(false, 0, 5); got != "-- / 5 min" {
		t.Fatalf("formatHysteresis() = %q, want %q", got, "-- / 5 min")
	}

	if got := formatHysteresis(true, 125, 5); got != "2:05 / 5 min" {
		t.Fatalf("formatHysteresis() = %q, want %q", got, "2:05 / 5 min")
	}
}

func TestFormatTemperatureOffset(t *testing.T) {
	if got := formatTemperatureOffset(-1); got != "-1.0 C" {
		t.Fatalf("formatTemperatureOffset(-1) = %q, want %q", got, "-1.0 C")
	}

	if got := formatTemperatureOffset(1); got != "+1.0 C" {
		t.Fatalf("formatTemperatureOffset(1) = %q, want %q", got, "+1.0 C")
	}
}

func TestFormatTemperature(t *testing.T) {
	if got := formatTemperature(nil); got != "--" {
		t.Fatalf("formatTemperature(nil) = %q, want %q", got, "--")
	}

	temp := 52.5
	if got := formatTemperature(&temp); got != "52.5 C" {
		t.Fatalf("formatTemperature(&temp) = %q, want %q", got, "52.5 C")
	}
}

func TestStatusStreamSetsSSEHeadersAndEmitsInitialEvent(t *testing.T) {
	state.SetEnergyStatus(&pv.PVData{Power: 1234.5, Soc: 67.8})

	req := httptest.NewRequest(http.MethodGet, "/api/status/stream", nil)
	writer := &streamingResponseWriter{failOnWrite: true}

	StatusStream(writer, req)

	if got := writer.Header().Get("Content-Type"); got != "text/event-stream; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want %q", got, "text/event-stream; charset=utf-8")
	}

	if got := writer.Header().Get("Cache-Control"); got != "no-cache, no-transform" {
		t.Fatalf("Cache-Control = %q, want %q", got, "no-cache, no-transform")
	}

	if got := writer.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Fatalf("X-Accel-Buffering = %q, want %q", got, "no")
	}

	if got := writer.body.String(); got == "" {
		t.Fatal("expected an initial SSE payload")
	}
}

func TestUpdateDaemonsPersistsCheckboxState(t *testing.T) {
	tempDir := t.TempDir()
	configPath = filepath.Join(tempDir, "config.json")
	t.Cleanup(func() {
		configPath = config.DefaultPath
		applyHotWaterDaemonState = func(enabled bool) error {
			return nil
		}
		applyHeatingDaemonState = func(enabled bool) error {
			return nil
		}
	})

	if err := (&config.Config{}).Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	var hotWaterEnabled, heatingEnabled bool
	applyHotWaterDaemonState = func(enabled bool) error {
		hotWaterEnabled = enabled
		return nil
	}
	applyHeatingDaemonState = func(enabled bool) error {
		heatingEnabled = enabled
		return nil
	}

	form := url.Values{}
	form.Set("hotWaterEnabled", "false")
	form.Set("heatingEnabled", "true")
	req := httptest.NewRequest(http.MethodPost, "/api/daemons", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	UpdateDaemons(recorder, req)

	if recorder.Code != http.StatusOK {
		body, _ := os.ReadFile(configPath)
		t.Fatalf("status = %d, want %d; body=%s config=%s", recorder.Code, http.StatusOK, recorder.Body.String(), string(body))
	}

	if hotWaterEnabled {
		t.Fatal("expected hot-water daemon to be disabled")
	}

	if !heatingEnabled {
		t.Fatal("expected heating daemon to be enabled")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HotWaterDaemonEnabled() {
		t.Fatal("expected hot-water daemon setting to persist as disabled")
	}

	if !cfg.HeatingDaemonEnabled() {
		t.Fatal("expected heating daemon setting to persist as enabled")
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/json")
	}
}

func TestUpdateThresholdsPersistsConfigValues(t *testing.T) {
	tempDir := t.TempDir()
	configPath = filepath.Join(tempDir, "config.json")
	t.Cleanup(func() {
		configPath = config.DefaultPath
	})

	if err := (&config.Config{}).Save(configPath); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	form := url.Values{}
	form.Set("hotWaterPower", "6100")
	form.Set("hotWaterSoc", "82")
	form.Set("hotWaterSwitchOnHysteresisMinutes", "7")
	form.Set("hotWaterSwitchOffHysteresisMinutes", "11")
	form.Set("hotWaterActivationCutoff", "15")
	form.Set("hotWaterExtraTemperature", "58.5")
	form.Set("heatingPower", "5400")
	form.Set("heatingSoc", "79")
	form.Set("heatingSwitchOnHysteresisMinutes", "4")
	form.Set("heatingSwitchOffHysteresisMinutes", "9")
	form.Set("heatingNormalOffset", "-0.5")
	form.Set("heatingPVOffset", "1.5")

	req := httptest.NewRequest(http.MethodPost, "/api/config/thresholds", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()

	UpdateThresholds(recorder, req)

	if recorder.Code != http.StatusOK {
		body, _ := os.ReadFile(configPath)
		t.Fatalf("status = %d, want %d; body=%s config=%s", recorder.Code, http.StatusOK, recorder.Body.String(), string(body))
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.ThresholdsHotWater.Power != 6100 {
		t.Fatalf("ThresholdsHotWater.Power = %v, want 6100", cfg.ThresholdsHotWater.Power)
	}

	if cfg.ThresholdsHotWater.SwitchOffHysteresisMinutes != 11 {
		t.Fatalf("ThresholdsHotWater.SwitchOffHysteresisMinutes = %d, want 11", cfg.ThresholdsHotWater.SwitchOffHysteresisMinutes)
	}

	if cfg.ThresholdsHotWater.ExtraHotWaterTemperature != 58.5 {
		t.Fatalf("ThresholdsHotWater.ExtraHotWaterTemperature = %v, want 58.5", cfg.ThresholdsHotWater.ExtraHotWaterTemperature)
	}

	if cfg.ThresholdsHeating.Power != 5400 {
		t.Fatalf("ThresholdsHeating.Power = %v, want 5400", cfg.ThresholdsHeating.Power)
	}

	if cfg.ThresholdsHeating.NormalOffset == nil || *cfg.ThresholdsHeating.NormalOffset != -0.5 {
		t.Fatalf("ThresholdsHeating.NormalOffset = %v, want -0.5", cfg.ThresholdsHeating.NormalOffset)
	}

	if cfg.ThresholdsHeating.PVOffset == nil || *cfg.ThresholdsHeating.PVOffset != 1.5 {
		t.Fatalf("ThresholdsHeating.PVOffset = %v, want 1.5", cfg.ThresholdsHeating.PVOffset)
	}

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/json")
	}
}
