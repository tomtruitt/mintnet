First, install `docker-machine` and get a DigitalOcean account and access token.

To create a testnet on DigitalOcean:

```
go run mintnet.go create --nodes 4 --gen-file-out=genesis.json -- --driver=digitalocean --digitalocean-access-token=YOUR_ACCCESS_TOKEN
```

This generates a bare genesis.json file with no accounts.  Update it manually.
When you're done, copy the genesis.json to the testnet nodes.

```
go run mintnet.go copy-genesis --nodes 4 --gen-file=genesis.json
```

If everything went well, your testnet should start generating blocks.

Remember to destroy the testnet when you're done.

```
go run mintnet.go destroy
```
