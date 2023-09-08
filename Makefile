.PHONY:	pre-ui
pre-ui:
	cd ui/teldrive-ui && npm ci

.PHONY:	ui
ui:	
	cd ui/teldrive-ui && npm run build

.PHONY: teldrive
teldrive:
	go build -ldflags "-s -w"