#! /bin/bash
set -eu

# NOTE: DIGITALOCEAN_ACCESS_TOKEN must be set!

mintnet create -- --driver=digitalocean --digitalocean-image=docker

mintnet init chain mytest_dir/

mintnet start mytest mytest_dir/

bash test/run_test.sh

bash test/destroy.sh my_test
