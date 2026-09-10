# Show recipes by default
default:
    @just --list

# Build every package
build:
    go build ./...

# Run the test suite
test:
    go test ./...

# Run tests with the race detector and coverage
test-cov:
    go test -race -cover ./...

# Static analysis
vet:
    go vet ./...

# Check formatting (non-zero exit if anything is unformatted)
fmt-check:
    test -z "$(gofmt -l .)" || (gofmt -l . && exit 1)

# Format in place
fmt:
    gofmt -w .

# Regenerate data/cpu_models.json + build/cpu_models.{sql,db} from live sources
ingest:
    go run ./cmd/cpuids-ingest

# Build the SQLite artifact from the committed data/cpu_models.json
sqlite:
    go run ./cmd/cpuids-sqlite

# Everything CI runs
ci: fmt-check vet test build

# Remove build outputs
clean:
    rm -rf build/
