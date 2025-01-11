ifdef ComSpec
SHELL := powershell.exe
IS_WINDOWS := true
else
SHELL := /bin/bash
IS_WINDOWS := false
endif

APP_NAME := teldrive
BUILD_DIR := bin
FRONTEND_DIR := ui/dist
FRONTEND_ASSET := https://github.com/tgdrive/teldrive-ui/releases/download/latest/teldrive-ui.zip
GIT_COMMIT := $(shell git rev-parse --short HEAD)
GIT_LINK := $(shell git remote get-url origin)
MODULE_PATH := $(shell go list -m)
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GIT_COMMIT := $(shell git rev-parse --short HEAD)
VERSION_PACKAGE := $(MODULE_PATH)/internal/version
BINARY_EXTENSION :=

ifeq ($(IS_WINDOWS),true)
    TAG_FILTER:=Sort-Object -Descending | Select-Object -First 1
else
    TAG_FILTER:=head -n 1
endif

GIT_TAG := $(shell git tag -l '[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | $(TAG_FILTER))

ifeq ($(IS_WINDOWS),true)
    BINARY_EXTENSION := .exe
    RM := powershell -Command "Remove-Item"
    RMDIR := powershell -Command "Remove-Item -Recurse -Force"
    MKDIR := powershell -Command "New-Item -ItemType Directory -Force"
    DOWNLOAD := powershell -Command "Invoke-WebRequest -Uri"
    UNZIP := powershell -Command "Expand-Archive"
else
    RM := rm -f
    RMDIR := rm -rf
    MKDIR := mkdir -p
    DOWNLOAD := curl -sLO
    UNZIP := unzip -q -d
endif

.PHONY: all build run clean frontend backend run sync-ui retag patch-version minor-version major-version gen check-semver install-semver

all: build

check-semver:
ifeq ($(IS_WINDOWS),true)
	@powershell -Command "if (-not (Get-Command semver -ErrorAction SilentlyContinue)) { Write-Host 'Installing semver...'; npm install -g semver }"
else
	@which semver > /dev/null || (echo "Installing semver..." && npm install -g semver)
endif

frontend:
	@echo "Extract UI"
	$(RMDIR) $(FRONTEND_DIR)
ifeq ($(IS_WINDOWS),true)
	$(DOWNLOAD) $(FRONTEND_ASSET) -OutFile teldrive-ui.zip
	$(MKDIR) $(subst /,\\,$(FRONTEND_DIR))
	$(UNZIP) -Path teldrive-ui.zip -DestinationPath $(FRONTEND_DIR) -Force
	$(RM) teldrive-ui.zip
else
	$(DOWNLOAD) $(FRONTEND_ASSET)
	$(MKDIR) $(FRONTEND_DIR)
	$(UNZIP) $(FRONTEND_DIR) teldrive-ui.zip
	$(RM) teldrive-ui.zip
endif

gen:
	go generate ./...

backend: gen
	@echo "Building backend for $(GOOS)/$(GOARCH)..."
	go build -trimpath -ldflags "-s -w -X '$(VERSION_PACKAGE).Version=$(GIT_TAG)' -X '$(VERSION_PACKAGE).CommitSHA=$(GIT_COMMIT)' -extldflags=-static" -o $(BUILD_DIR)/$(APP_NAME)$(BINARY_EXTENSION)


build: frontend backend
	@echo "Building complete."

run:
	@echo "Running $(APP_NAME)..."
	$(BUILD_DIR)/$(APP_NAME) run

clean:
	@echo "Cleaning up..."
	$(RMDIR) $(BUILD_DIR)
ifeq ($(IS_WINDOWS),true)
	if exist "$(FRONTEND_DIR)" $(RMDIR) "$(FRONTEND_DIR)"
else
	$(RMDIR) $(FRONTEND_DIR)
endif

deps:
	@echo "Installing Go dependencies..."
	go mod download

retag:
	@echo "Retagging $(GIT_TAG)..."
	-git tag -d $(GIT_TAG)
	-git push --delete origin $(GIT_TAG)
	git tag -a $(GIT_TAG) -m "Recreated tag $(GIT_TAG)"
	git push origin $(GIT_TAG)

patch-version: check-semver
	@echo "Current version: $(GIT_TAG)"
ifeq ($(GIT_TAG),)
	$(eval NEW_VERSION := 1.0.0)
else
	$(eval NEW_VERSION := $(shell semver -i patch $(GIT_TAG)))
endif
	@echo "Creating new patch version: $(NEW_VERSION)"
	git tag -a $(NEW_VERSION) -m "Release $(NEW_VERSION)"
	git push origin $(NEW_VERSION)

minor-version: check-semver
	@echo "Current version: $(GIT_TAG)"
ifeq ($(GIT_TAG),)
	$(eval NEW_VERSION := 1.0.0)
else
	$(eval NEW_VERSION := $(shell semver -i minor $(GIT_TAG)))
endif
	@echo "Creating new minor version: $(NEW_VERSION)"
	git tag -a $(NEW_VERSION) -m "Release $(NEW_VERSION)"
	git push origin $(NEW_VERSION)

major-version: check-semver
	@echo "Current version: $(GIT_TAG)"
ifeq ($(GIT_TAG),)
	$(eval NEW_VERSION := 1.0.0)
else
	$(eval NEW_VERSION := $(shell semver -i major $(GIT_TAG)))
endif
	@echo "Creating new major version: $(NEW_VERSION)"
	git tag -a $(NEW_VERSION) -m "Release $(NEW_VERSION)"
	git push origin $(NEW_VERSION)