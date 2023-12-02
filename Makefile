.PHONY:	pre-ui
pre-ui:
	cd ui/teldrive-ui && npm ci

.PHONY:	ui
ui:	
	cd ui/teldrive-ui && npm run build

.PHONY: sync-ui
sync-ui:
	git submodule update --init --recursive --remote
	

.PHONY: teldrive
teldrive:
	go build -trimpath -ldflags "-s -w -extldflags=-static" cmd/teldrive/main.go