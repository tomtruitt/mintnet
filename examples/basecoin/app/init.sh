#! /bin/bash

go get github.com/tendermint/basecoin/cmd/...

basecoin --eyes="unix:///data/tendermint/data/data.sock" --genesis="/data/tendermint/app/genesis.json"
