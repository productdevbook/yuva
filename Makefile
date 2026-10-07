.PHONY: build web

build: web
	@if [ -f web/dist/index.html ]; then \
		find api/internal/ui/dist -mindepth 1 ! -name .gitignore -delete && cp -R web/dist/. api/internal/ui/dist/; \
	fi
	cd api && CGO_ENABLED=0 go build -trimpath -o ../bin/yuva ./cmd/yuva

web:
	@if [ -f web/package.json ]; then \
		cd web && { [ -d node_modules ] || npm ci; } && npm run build; \
	fi
