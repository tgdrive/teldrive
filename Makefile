.PHONY:	pre-ui
pre-ui:
	cd ui/teldrive-ui && npm ci

.PHONY:	ui
ui:	
	cd ui/teldrive-ui && npm run build

.PHONY: sync-ui
sync-ui:
	git submodule update --remote --rebase
	

.PHONY: teldrive
teldrive:
	go build -ldflags "-s -w"