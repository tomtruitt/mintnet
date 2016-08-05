#! /bin/bash
set -e

if [[ "$MACH_PREFIX" == "" ]]; then
	MACH_PREFIX=mach
fi

if [[ "$N" == "" ]]; then
	N=4
fi

set -u

echo "Stopping and removing testnet containers"
mintnet stop mytest; mintnet rm mytest

echo "Destroying machines"
mintnet destroy --machines="${MACH_PREFIX}[1-$N]"
