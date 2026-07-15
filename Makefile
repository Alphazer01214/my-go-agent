
run:
	go run main.go

run-mygo:
	go run cmd/mygoagent/main.go

run-agent:
	go run cmd/agent/main.go

test:
	go test -v ./...

build:
	go build -o bin/mygoagent cmd/mygoagent/main.go
