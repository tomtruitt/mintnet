.PHONY: all test get_deps

all: test get_deps install

install:
	go install github.com/tendermint/mintnet

test:
	go test github.com/tendermint/mintnet/...

get_deps:
	go get -d github.com/tendermint/mintnet/...
