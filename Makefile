APP=gosk
VERSION=$(shell git describe --always --dirty)

build:
	go build -ldflags "-s -X main.version=${VERSION}" -o ${APP} .

run:
	go run -race main.go

clean:
	go clean
