APP=gosk

build:
	go build -o ${APP} main.go

run:
	go run -race main.go

clean:
	go clean
