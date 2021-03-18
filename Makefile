APP=gosk

build:
	protoc --go_out=paths=source_relative:. ./nanomsg/messages.proto
	go build -o ${APP} .

run:
	go run -race main.go

clean:
	go clean
