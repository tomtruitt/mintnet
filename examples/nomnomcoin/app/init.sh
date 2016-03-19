#! /bin/bash
# This is a sample bash script for a TMSP application

cd app/
git clone https://github.com/tendermint/nomnomcoin.git
cd nomnomcoin
npm install .

node app.js --eyes="unix:///data/tendermint/data/data.sock" --addr="unix:///data/tendermint/app/app.sock"
