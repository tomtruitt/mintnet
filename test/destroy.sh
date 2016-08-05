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
mintnet stop --machines="${MACH_PREFIX}[1-$N]" mytest
mintnet rm --machines="${MACH_PREFIX}[1-$N]" mytest

echo "Destroying machines"
mintnet destroy --machines="${MACH_PREFIX}[1-$N]"
