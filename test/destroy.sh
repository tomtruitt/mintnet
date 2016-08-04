#! /bin/bash
set -eu

echo "Stopping and removing testnet containers"
mintnet stop mytest; mintnet rm mytest

echo "Destroying machines"
mintnet destroy --machines="mach[1-4]"
