#! /bin/bash
set -e

if [[ "$MACH_PREFIX" == "" ]]; then
	MACH_PREFIX=mach
fi

if [[ "$N" == "" ]]; then
	N=4
fi

BRANCH=`git rev-parse --abbrev-ref HEAD`
echo "Current branch: $BRANCH"

if [[ "$BRANCH" == "master" || "$BRANCH" == "staging" ]]; then
	# NOTE: DIGITALOCEAN_ACCESS_TOKEN must be set!

	mintnet create --machines "${MACH_PREFIX}[1-$N]" -- --driver=digitalocean --digitalocean-image=docker

	mintnet init --machines "${MACH_PREFIX}[1-$N]" chain mytest_dir/

	mintnet start --machines "${MACH_PREFIX}[1-$N]" mytest mytest_dir/

	bash test/run_test.sh

	bash test/destroy.sh my_test
fi
