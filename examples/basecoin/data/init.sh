#! /bin/bash
# This is a sample bash script for MerkleEyes.
# https://github.com/tendermint/merkleeyes

go get github.com/tendermint/merkleeyes/cmd/merkleeyes

merkleeyes server --address="unix:///data/tendermint/data/data.sock"
