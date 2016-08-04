.PHONY: get_deps build all install

all: get_deps install test

TMROOT = $${TMROOT:-$$HOME/.tendermint}
define NEWLINE

endef
NOVENDOR = go list github.com/tendermint/mintnet/... | grep -v /vendor/

install: get_deps
	go install github.com/tendermint/mintnet

test: 
	go test `${NOVENDOR}`
	
test_race: 
	go test -race `${NOVENDOR}`

test_integrations: test_race
	bash ./test/test.sh

get_deps:
	go get -d `${NOVENDOR}`
	go list -f '{{join .TestImports "\n"}}' github.com/tendermint/mintnet/... | \
		grep -v /vendor/ | sort | uniq | \
		xargs go get

update_deps:
	go get -d -u github.com/tendermint/mintnet

