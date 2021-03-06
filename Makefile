.PHONY: build test test-junit install

build:
	mkdir -p bin
	go build -o bin/ -v ./...

test: build
	go test -v ./...

test-junit: build
	go get -u github.com/jstemmer/go-junit-report
	go test -v ./... 2>&1 | go-junit-report > report.xml

install: test
	go install
