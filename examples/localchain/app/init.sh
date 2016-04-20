#! /bin/bash

go get github.com/tendermint/basecoin/cmd/...
cd $GOPATH/src/github.com/tendermint/basecoin
git fetch -a origin
git checkout -b demo
git reset --hard origin/demo
make install

basecoin --address="unix:///data/tendermint/app/app.sock" --eyes="unix:///data/tendermint/data/data.sock" --genesis="/data/tendermint/app/genesis.json"
