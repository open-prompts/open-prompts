.PHONY: ut fvt image-build lint check

# Run Unit Tests
ut:
	go test ./...

# Run Functional Verification Tests
# Assumes docker-compose is installed and available
fvt:
	cd .. && python3 tests/fvt.py

# Build Docker Image
image-build:
	docker build -t open-prompts-backend .

# Run Linters
lint:
	golangci-lint run

# Run All Checks
check: lint ut fvt
