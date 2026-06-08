.PHONY: all build run clean

all: build

build:
	@echo "Building onet..."
	@mkdir -p bin
	go build -o bin/onet .
	@echo "Build complete: bin/onet"

run: build
	@echo "Running onet in foreground..."
	sudo ./bin/onet orun

clean:
	@echo "Cleaning up..."
	rm -rf bin
	@echo "Clean complete."
