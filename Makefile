FRONTEND_DIR := ui/teldrive-ui
BUILD_DIR := bin
APP_NAME := teldrive

GIT_VERSION := $(shell git describe --tags --abbrev=0 --exclude='*dev*')
GIT_COMMIT := $(shell git rev-parse --short HEAD)
GIT_LINK := $(shell git remote get-url origin)
GIT_DEV_TAG := $(shell git describe --tags --abbrev=0 --match='*-dev')
ENV_FILE := $(FRONTEND_DIR)/.env
MODULE_PATH := $(shell go list -m)

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

.PHONY: all build run clean frontend backend run sync-ui tag-and-push retag dev-tag dev-retag
 
all: build

ifdef ComSpec
SHELL := powershell.exe
BINARY_EXTENSION:=.exe
else
SHELL := /bin/bash
BINARY_EXTENSION:=
endif

frontend: $(ENV_FILE)
	@echo "Building frontend..."
	cd $(FRONTEND_DIR) ; pnpm install ; pnpm build
	
$(ENV_FILE): Makefile
ifdef ComSpec
	@echo 'VITE_VERSION_INFO={"version": "$(GIT_VERSION)", "commit": "$(GIT_COMMIT)", "link": "$(GIT_LINK)"}' | Out-File -encoding utf8 $(ENV_FILE)
else
	@echo 'VITE_VERSION_INFO={"version": "$(GIT_VERSION)", "commit": "$(GIT_COMMIT)", "link": "$(GIT_LINK)"}' > $(ENV_FILE)
endif

backend:
	@echo "Building backend for $(GOOS)/$(GOARCH)..."
	go build -trimpath -ldflags "-s -w -X $(MODULE_PATH)/internal/config.Version=$(GIT_VERSION) -extldflags=-static" -o $(BUILD_DIR)/$(APP_NAME)$(BINARY_EXTENSION)

build: frontend backend
	@echo "Building complete."

run:
	@echo "Running $(APP_NAME)..."
	$(BUILD_DIR)/$(APP_NAME) run

clean:
	@echo "Cleaning up..."
	rm -rf $(BUILD_DIR)
	cd $(FRONTEND_DIR) && rm -rf dist node_modules

deps:
	@echo "Installing Go dependencies..."
	go mod download

	@echo "Installing frontend dependencies..."
	cd $(FRONTEND_DIR) && pnpm install

sync-ui:
	git submodule update --init --recursive --remote
	
retag:
	@echo "Retagging..."
	git tag -d $(GIT_VERSION)
	git push --delete origin $(GIT_VERSION)
	git tag -a $(GIT_VERSION) -m "Recreated tag $(GIT_VERSION)"
	git push origin $(GIT_VERSION)

patch-version:
	@echo "Patching version..."
	git tag -a $(shell semver -i patch $(GIT_VERSION)) -m "Release $(shell semver -i patch $(GIT_VERSION))"
	git push origin $(shell semver -i patch $(GIT_VERSION))

minor-version:
	@echo "Minoring version..."
	git tag -a $(shell semver -i minor $(GIT_VERSION)) -m "Release $(shell semver -i minor $(GIT_VERSION))"
	git push origin $(shell semver -i minor $(GIT_VERSION))

dev-patch:
	@echo "Patching version..."
	git tag $(shell semver -i patch $(GIT_DEV_TAG))
	git push origin $(shell semver -i patch $(GIT_DEV_TAG))
	
dev-retag:
	@echo "Retagging..."
	git tag -d $(GIT_DEV_TAG)
	git push --delete origin $(GIT_DEV_TAG)
	git tag $(GIT_DEV_TAG)
	git push origin $(GIT_DEV_TAG)