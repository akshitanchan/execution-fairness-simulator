.PHONY: build test lint run-calm run-thin run-spike demo report clean

BINARY := fairsim
PKG := ./cmd/fairsim

build:
	go build -o $(BINARY) $(PKG)

test:
	go test -v -race -count=1 ./...

test-short:
	go test -short -race ./...

lint:
	go vet ./...

run-calm: build
	./$(BINARY) run --scenario calm --seed 42

run-thin: build
	./$(BINARY) run --scenario thin --seed 42

run-spike: build
	./$(BINARY) run --scenario spike --seed 42

demo: build
	@echo "=== Running all scenarios with consolidated report ==="
	./$(BINARY) demo --seed 42

report: build
	./$(BINARY) report --last-run

clean:
	rm -f $(BINARY)
	rm -rf runs/
