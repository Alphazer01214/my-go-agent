
run:
	go run main.go

run-mygo:
	go run ./cmd/mygoagent

run-agent:
	go run ./cmd/agent

test:
	go test -v ./...

build:
	go build -o bin/mygoagent.exe ./cmd/mygoagent
