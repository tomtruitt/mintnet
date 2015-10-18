First, get a DigitalOcean account and access token.

To create a testnet on DigitalOcean:

```
go run mintnet.go create --nodes 4 -- --driver=digitalocean --digitalocean-access-token=YOUR_ACCCESS_TOKEN
```

To destroy the testnet:

```
go run mintnet.go destroy
```
