package controller

import (
	"fmt"
	"html/template"
	"net/http"
	"pv_hp_ctrl/pkg/state"
)

const statusTemplate = `
<!DOCTYPE html>
<html>
<head>
    <title>PV Control Status</title>
    <style>
        body { font-family: sans-serif; }
        .status { padding: 1em; border-radius: 5px; color: white; }
        .active { background-color: #4CAF50; }
        .inactive { background-color: #f44336; }
    </style>
</head>
<body>
    <h1>PV Control Status</h1>
    <p>Last Check: {{.LastCheck.Format "2006-01-02 15:04:05"}}</p>
    <div class="status {{if .IsActive}}active{{else}}inactive{{end}}">
        <p>{{.Message}}</p>
    </div>
    {{if .PVData}}
    <h2>PV Data</h2>
    <p>PV Leistung aktuell: {{printf "%.2f" .PVData.Power}} W</p>
    <p>Verbrauch aktuell: {{printf "%.2f" .PVData.Consumption}} W</p>
    <p>Ladestand Akku: {{printf "%.2f" .PVData.Soc}} %</p>
    {{end}}
</body>
</html>
`

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprint(w, "OK")
}

func StatusPage(w http.ResponseWriter, r *http.Request) {
	tmpl, err := template.New("status").Parse(statusTemplate)
	if err != nil {
		http.Error(w, "Failed to parse template", http.StatusInternalServerError)
		return
	}

	status := state.GetStatus()
	err = tmpl.Execute(w, status)
	if err != nil {
		http.Error(w, "Failed to execute template", http.StatusInternalServerError)
		return
	}
}
