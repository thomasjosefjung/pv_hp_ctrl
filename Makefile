.PHONY: up down image renew

renew: image up

image: 
	docker build . -t pv_hp_ctrl:latest

restart: down up 

up:
	cd /opt/homeautomation && docker compose up -d

down:
	cd /opt/homeautomation && docker compose down
	
