.PHONY: help build css css-watch clean install-tailwind

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: css ## Build the application
	go build -o bin/go-odio-api

css: ## Build CSS (production)
	@if [ ! -f bin/tailwindcss ]; then \
		echo "Tailwind CLI not found. Run 'make install-tailwind' first."; \
		exit 1; \
	fi
	bin/tailwindcss -i ui/styles/input.css -o ui/static/output.css --minify

css-watch: ## Build CSS and watch for changes (development)
	@if [ ! -f bin/tailwindcss ]; then \
		echo "Tailwind CLI not found. Run 'make install-tailwind' first."; \
		exit 1; \
	fi
	bin/tailwindcss -i ui/styles/input.css -o ui/static/output.css --watch

install-tailwind: ## Download Tailwind CLI standalone binary (v3)
	@mkdir -p bin
	@echo "Downloading Tailwind CLI v3..."
	@curl -sL https://github.com/tailwindlabs/tailwindcss/releases/download/v3.4.17/tailwindcss-linux-x64 -o bin/tailwindcss
	@chmod +x bin/tailwindcss
	@echo "Tailwind CLI v3 installed to bin/tailwindcss"

clean: ## Clean build artifacts
	rm -rf bin/go-odio-api ui/static/output.css
