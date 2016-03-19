#! /bin/bash

go get github.com/tendermint/basecoin/cmd/...

basecoin --addr="unix:///data/tendermint/app/app.sock" --eyes="unix:///data/tendermint/data/data.sock" --genesis="/data/tendermint/app/genesis.json"
