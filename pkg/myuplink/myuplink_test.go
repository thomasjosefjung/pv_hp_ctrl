package myuplink

import "testing"

func TestOperationModeText(t *testing.T) {
	tests := []struct {
		name  string
		value OperationModeValue
		want  string
	}{
		{name: "heating operation", value: OperationModeHeatingOperation, want: "heating operation"},
		{name: "domestic hot water", value: OperationModeDomesticHotWater, want: "domestic hot water"},
		{name: "swimming pool", value: OperationModeSwimmingPool, want: "swimming pool"},
		{name: "evu cut-off time", value: OperationModeEVUCutOffTime, want: "evu cut-off time"},
		{name: "forced defrosting", value: OperationModeForcedDefrosting, want: "forced defrosting"},
		{name: "no request", value: OperationModeNoRequest, want: "no request"},
		{name: "heat external energy source", value: OperationModeHeatExtEnergySource, want: "heat.ext.energ.source"},
		{name: "cooling mode", value: OperationModeCoolingMode, want: "cooling mode"},
		{name: "unknown", value: OperationModeValue(99), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := operationModeText(test.value); got != test.want {
				t.Fatalf("operationModeText(%v) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestOperationModeOptions(t *testing.T) {
	if OperationModeOptions.DomesticHotWater != OperationModeDomesticHotWater {
		t.Fatalf("OperationModeOptions.DomesticHotWater = %v, want %v", OperationModeOptions.DomesticHotWater, OperationModeDomesticHotWater)
	}
}

func TestHeatingModeText(t *testing.T) {
	tests := []struct {
		name  string
		value HeatingModeValue
		want  string
	}{
		{name: "automatic", value: HeatingModeAutomatic, want: "automatic"},
		{name: "additional heat", value: HeatingModeAdditionalHeat, want: "add. heat gen."},
		{name: "party", value: HeatingModeParty, want: "party"},
		{name: "holiday", value: HeatingModeHoliday, want: "holiday"},
		{name: "off", value: HeatingModeOff, want: "Off"},
		{name: "unknown", value: HeatingModeValue(99), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := heatingModeText(test.value); got != test.want {
				t.Fatalf("heatingModeText(%v) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestHeatingModeOptions(t *testing.T) {
	if HeatingModeOptions.Automatic != HeatingModeAutomatic {
		t.Fatalf("HeatingModeOptions.Automatic = %v, want %v", HeatingModeOptions.Automatic, HeatingModeAutomatic)
	}
}
