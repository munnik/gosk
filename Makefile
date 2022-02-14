APP=gosk

build:
	go build -o ${APP} .

run:
	go run -race main.go

clean:
	go clean
