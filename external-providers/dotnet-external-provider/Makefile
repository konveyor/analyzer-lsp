OS ?= $(shell go env GOOS)
ARCH ?= $(shell go env GOARCH)
DEFAULT_BIN := dotnet-external-provider
ADD_PLAT ?= no

ifeq ($(ADD_PLAT),yes)
	DEFAULT_BIN := $(DEFAULT_BIN)-$(OS)-$(ARCH)
endif


ifeq ($(OS),windows)
	BIN ?= $(DEFAULT_BIN).exe
else
	BIN ?= $(DEFAULT_BIN)
endif

build:
	CGO_ENABLED=0 GOOS=$(OS) GOARCH=$(ARCH) go build -o bin/$(BIN) main.go

.PHONY: clean
clean:
	@rm -rf bin/
