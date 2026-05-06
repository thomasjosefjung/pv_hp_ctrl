package main

import (
	"log"
	"net/http"
	"pv_hp_ctrl/pkg/controller"
	"pv_hp_ctrl/pkg/daemoncore"
	"pv_hp_ctrl/pkg/energydaemon"
	"pv_hp_ctrl/pkg/heatingdaemon"
	"pv_hp_ctrl/pkg/hotwaterdaemon"
)

func main() {
	go daemoncore.Run(
		energydaemon.RunTask,
		hotwaterdaemon.RunTask,
		heatingdaemon.RunTask,
	)

	http.HandleFunc("/", controller.StatusPage)
	http.HandleFunc("/api/status", controller.StatusAPI)
	http.HandleFunc("/api/daemons", controller.UpdateDaemons)
	http.HandleFunc("/api/config/thresholds", controller.UpdateThresholds)
	http.HandleFunc("/api/status/stream", controller.StatusStream)
	http.HandleFunc("/health", controller.HealthCheck)
	log.Fatal(http.ListenAndServe(":8081", nil))
}
