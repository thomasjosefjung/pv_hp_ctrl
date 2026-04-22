package controller

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	if response.Heating.SwitchOffHysteresisMinutes != 10 {
		t.Fatalf("Heating.SwitchOffHysteresisMinutes = %d, want 10", response.Heating.SwitchOffHysteresisMinutes)
	}

	if response.Heating.NormalOffset != -1.0 {
		t.Fatalf("Heating.NormalOffset = %v, want -1.0", response.Heating.NormalOffset)
	}

	if response.Heating.PVOffset != 1.0 {
		t.Fatalf("Heating.PVOffset = %v, want 1.0", response.Heating.PVOffset)
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
