package main

import (
	"log"
	"net/http"
	"pv_hp_ctrl/pkg/controller"
	"pv_hp_ctrl/pkg/daemon"
)

func main() {
	go daemon.Run()

	http.HandleFunc("/", controller.StatusPage)
	http.HandleFunc("/health", controller.HealthCheck)
	log.Fatal(http.ListenAndServe(":8081", nil))
}
