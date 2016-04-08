First, install [`docker-machine`](https://docs.docker.com/machine/install-machine/) and get a DigitalOcean account and access token.

Then, install `mintnet`.

```bash
go get github.com/tendermint/mintnet
```

To create machines on DigitalOcean:

```bash
mintnet create -- --driver=digitalocean --digitalocean-access-token=YOUR_ACCCESS_TOKEN
```
**Note:** adding `export DIGITALOCEAN_ACCESS_TOKEN=your_access_token` to your `~/.bashrc` will make your life easier when frequently creating testnets.

Or, to create machines on AWS:

```bash
mintnet create -- --driver=amazonec2 --amazonec2-access-key=AKI******* --amazonec2-secret-key=8T93C********* --amazonec2-zone=b --amazonec2-vpc-id=vpc-****** aws01`
```

**Note:** Similarly these environment variables can be added to `~/.bashrc`.

```bash
export AWS_ACCESS_KEY_ID=your_id\
AWS_DEFAULT_REGION=your_region\
AWS_SECRET_ACCESS_KEY=your_key\
AWS_SECURITY_GROUP=your_group\
AWS_VPC_ID=your_vpc
```
By default this creates 4 new machines.  Check the help messages for more info, e.g. `mintnet create --help`.

Next, create the testnet configuration folders.

```bash
mintnet init chain mytest_dir/
```

This creates directories in `mytest_dir` for the application.

```bash
ls mytest_dir/
  app   # Common configuration directory for your blockchain applicaiton
  core  # Common configuration directory for Tendermint Core
  data  # Common configuration directory for MerkleEyes key-value store
  mach1 # Configuration directory for the Tendermint core daemon on machine 1
  mach2 # Configuration directory for the Tendermint core daemon on machine 2
  mach3 # Configuration directory for the Tendermint core daemon on machine 3
  mach4 # Configuration directory for the Tendermint core daemon on machine 4
```

Now start the testnet service.

```bash
mintnet start mytest mytest_dir/
```

You can stop and remove the application as well.

```bash
mintnet rm --force mytest
```

Don't forget to destroy your createed machines!

```bash
mintnet destroy --machines="mach1,mach2,mach3,mach4"
```
