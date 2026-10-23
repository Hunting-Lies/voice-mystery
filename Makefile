.PHONY: run test demo tidy

run:
	go run ./cmd/server -addr :8080 -web ./web

test:
	go test ./...

tidy:
	go mod tidy

demo:
	bash ./scripts/demo.sh
