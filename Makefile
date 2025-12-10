.PHONY: build run clean install help


BINARY_NAME=laninha-tiny-png
BUILD_DIR=bin
SOURCE_DIR=src

help:
	@echo "Available commands:"
	@echo "  make build    - Build the application"
	@echo "  make run      - Run the application (requires argument: make run ARGS='./folder')"
	@echo "  make install  - Install the application system-wide"
	@echo "  make clean    - Remove compiled files"
	@echo "  make help     - Show this message"

build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	@go build -o $(BUILD_DIR)/$(BINARY_NAME) $(SOURCE_DIR)/main.go
	@echo "Build complete! Binary at $(BUILD_DIR)/$(BINARY_NAME)"

run: build
	@if [ -z "$(ARGS)" ]; then \
		echo "Error: Specify folder path. Example: make run ARGS='./images'"; \
		exit 1; \
	fi
	@echo "Running $(BINARY_NAME)..."
	@./$(BUILD_DIR)/$(BINARY_NAME) $(ARGS)

install: build
	@echo "Installing $(BINARY_NAME)..."
	@sudo cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/
	@echo "Installed at /usr/local/bin/$(BINARY_NAME)"

clean:
	@echo "Cleaning compiled files..."
	@rm -rf $(BUILD_DIR)
	@echo "Cleanup complete!"

