APP     := unfolder
SRC     := unfolder.go
DIST    := dist

# Optional build metadata
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X 'main.version=$(VERSION)' -X 'main.commit=$(COMMIT)' -X 'main.date=$(DATE)'

# Target platforms
PLATFORMS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: build package checksum clean

build:
	@set -e; \
	for plat in $(PLATFORMS); do \
		os=$${plat%/*}; arch=$${plat#*/}; \
		ext=""; if [ "$$os" = "windows" ]; then ext=".exe"; fi; \
		outdir="$(DIST)/$(APP)_$${os}_$${arch}"; \
		outbin="$$outdir/$(APP)$$ext"; \
		echo "==> Building $$os/$$arch -> $$outbin"; \
		mkdir -p "$$outdir"; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -trimpath -ldflags "$(LDFLAGS)" -o "$$outbin" $(SRC); \
		chmod +x "$$outbin" 2>/dev/null || true; \
	done

package: build
	echo "==> Packaging artifacts"
	@cd $(DIST) && for d in $(APP)_*; do \
		if [ -f "$$d/$(APP).exe" ]; then \
			zip -q "$$d.zip" "$$d/$(APP).exe" && echo "   zipped $$d.zip"; \
		else \
			tar -czf "$$d.tar.gz" -C "$$d" "$(APP)" && echo "   tarred $$d.tar.gz"; \
		fi; \
	done

checksum: package
	echo "==> Generating SHA256SUMS"
	@cd $(DIST) && { \
		if command -v shasum >/dev/null 2>&1; then shasum -a 256 *.{zip,tar.gz}; \
		elif command -v sha256sum >/dev/null 2>&1; then sha256sum *.{zip,tar.gz}; \
		else echo "No shasum/sha256sum found" >&2; exit 1; fi; \
	} | tee "$(DIST)/SHA256SUMS" >/dev/null

clean:
	rm -rf "$(DIST)"