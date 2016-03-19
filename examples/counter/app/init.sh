#! /bin/bash
# This is a sample bash script for a TMSP application

BRANCH="master"

go get -d github.com/tendermint/tmsp/cmd/counter
cd $GOPATH/src/github.com/tendermint/tmsp/
git fetch origin $BRANCH
git checkout $BRANCH
make install

counter --serial --addr="unix:///data/tendermint/app/app.sock"
