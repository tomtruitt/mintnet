package main

import (
	"encoding/hex"
	"fmt"
	"path"
	"time"

	"github.com/codegangsta/cli"
	. "github.com/tendermint/go-common"
	"github.com/tendermint/go-wire"
	tmtypes "github.com/tendermint/tendermint/types"
)

//--------------------------------------------------------------------------------

func cmdInit(c *cli.Context) {
	cli.ShowAppHelp(c)
}

// initialize a new validator set
func cmdValidatorsInit(c *cli.Context) {
	args := c.Args()
	if len(args) != 1 {
		cli.ShowAppHelp(c)
		return
	}
	base := args[0]

	N := c.Int("N")
	vals := make([]*Validator, N)

	// Initialize priv_validator.json's
	for i := 0; i < N; i++ {
		err := initValDirectory(base, i)
		if err != nil {
			Exit(err.Error())
		}
		// Read priv_validator.json to populate vals
		name := fmt.Sprintf("val%d", i)
		privValFile := path.Join(base, name, "priv_validator.json")
		privVal := tmtypes.LoadPrivValidator(privValFile)
		vals[i] = &Validator{
			ID:     name,
			PubKey: privVal.PubKey,
		}
	}

	valSet := ValidatorSet{
		ID:         path.Base(base),
		Validators: vals,
	}
	// write the validator set file
	b := wire.JSONBytes(valSet)

	err := WriteFile(path.Join(base, "validator_set.json"), b, 0444)
	if err != nil {
		Exit(err.Error())
	}

	fmt.Println(Fmt("Successfully initialized %v validators", N))
}

// Initialize directories for each node
func cmdChainInit(c *cli.Context) {
	args := c.Args()
	if len(args) != 1 {
		cli.ShowAppHelp(c)
		return
	}
	base := args[0]
	machines := ParseMachines(c.GlobalString("machines"))
	app := c.String("app")

	var appHash []byte
	appHashString := c.String("app-hash")
	if appHashString != "" {
		if len(appHashString) >= 2 && appHashString[:2] == "0x" {
			var err error
			appHash, err = hex.DecodeString(appHashString[2:])
			if err != nil {
				Exit(err.Error())
			}
		} else {
			appHash = []byte(appHashString)
		}
	}

	err := initDataDirectory(base)
	if err != nil {
		Exit(err.Error())
	}
	err = initAppDirectory(base, app)
	if err != nil {
		Exit(err.Error())
	}
	err = initCoreDirectory(base)
	if err != nil {
		Exit(err.Error())
	}

	genVals := make([]tmtypes.GenesisValidator, len(machines))

	var valSetID string
	valSetDir := c.String("validator-set")
	if valSetDir != "" {
		// validator-set name is the last element of the path
		valSetID = path.Base(valSetDir)

		var valSet ValidatorSet
		err := ReadJSONFile(&valSet, path.Join(valSetDir, "validator_set.json"))
		if err != nil {
			Exit(err.Error())
		}
		vals := valSet.Validators

		if len(machines) != len(vals) {
			Exit(fmt.Sprintf("Validator set size must match number of machines. Got %d validators, %d machines", len(vals), len(machines)))
		}

		for i, val := range vals {

			// build the directory
			mach := machines[i]
			err := initMachCoreDirectory(base, mach)
			if err != nil {
				Exit(err.Error())
			}

			// overwrite the priv validator
			privValFile := path.Join(valSetDir, val.ID, "priv_validator.json")
			privVal := tmtypes.LoadPrivValidator(privValFile)
			privVal.SetFile(path.Join(base, mach, "core", "priv_validator.json"))
			privVal.Save()
		}

		// copy the vals into genVals
		for i, val := range vals {
			genVals[i] = tmtypes.GenesisValidator{
				Name:   val.ID,
				PubKey: val.PubKey,
				Amount: 1, // TODO
			}
		}
	} else {
		valSetID = ValSetAnon

		// Initialize core dir and priv_validator.json's
		for i, mach := range machines {
			err := initMachCoreDirectory(base, mach)
			if err != nil {
				Exit(err.Error())
			}
			// Read priv_validator.json to populate vals
			privValFile := path.Join(base, mach, "core", "priv_validator.json")
			privVal := tmtypes.LoadPrivValidator(privValFile)
			genVals[i] = tmtypes.GenesisValidator{
				PubKey: privVal.PubKey,
				Amount: 1,
				Name:   mach,
			}
		}
	}

	// Generate genesis doc from generated validators
	genDoc := &tmtypes.GenesisDoc{
		GenesisTime: time.Now(),
		ChainID:     "chain-" + RandStr(6),
		Validators:  genVals,
		AppHash:     appHash,
	}

	// Write genesis file.
	for _, mach := range machines {
		genDoc.SaveAs(path.Join(base, mach, "core", "genesis.json"))
	}

	// write the chain meta data (ie. validator set name and validators)
	blockchainCfg := &BlockchainConfig{
		ValSetID:   valSetID,
		Validators: make([]*ValidatorConfig, len(genVals)),
	}

	for i, v := range genVals {
		blockchainCfg.Validators[i] = &ValidatorConfig{
			Validator: &Validator{ID: v.Name, PubKey: v.PubKey},
			Index:     i, // XXX: we may want more control here
		}
	}
	err = WriteBlockchainConfig(base, blockchainCfg)
	if err != nil {
		Exit(err.Error())
	}

	fmt.Println(Fmt("Successfully initialized %v node directories", len(machines)))
}

// Initialize per-machine core directory
func initMachCoreDirectory(base, mach string) error {
	dir := path.Join(base, mach, "core")
	err := EnsureDir(dir, 0777)
	if err != nil {
		return err
	}

	// Create priv_validator.json file if not present
	ensurePrivValidator(path.Join(dir, "priv_validator.json"))
	return nil

}

func initValDirectory(base string, i int) error {
	name := fmt.Sprintf("val%d", i)
	dir := path.Join(base, name)
	err := EnsureDir(dir, 0777)
	if err != nil {
		return err
	}

	// Create priv_validator.json file if not present
	ensurePrivValidator(path.Join(dir, "priv_validator.json"))
	return nil
}

func ensurePrivValidator(file string) {
	if FileExists(file) {
		return
	}
	privValidator := tmtypes.GenPrivValidator()
	privValidator.SetFile(file)
	privValidator.Save()
}

// Initialize common data directory
func initDataDirectory(base string) error {
	dir := path.Join(base, "data")
	err := EnsureDir(dir, 0777)
	if err != nil {
		return err
	}

	// Write a silly sample bash script.
	scriptBytes := []byte(`#! /bin/bash
# This is a sample bash script for MerkleEyes.
# NOTE: mintnet expects data.sock to be created

go get github.com/tendermint/merkleeyes/cmd/merkleeyes

merkleeyes server --address="unix:///data/tendermint/data/data.sock"`)

	err = WriteFile(path.Join(dir, "init.sh"), scriptBytes, 0777)
	return err
}

// Initialize common app directory
func initAppDirectory(base, app string) error {
	dir := path.Join(base, "app")
	err := EnsureDir(dir, 0777)
	if err != nil {
		return err
	}

	var scriptBytes []byte
	if app == "" {
		// Write a silly sample bash script.
		scriptBytes = []byte(`#! /bin/bash
# This is a sample bash script for a TMSP application

cd app/
git clone https://github.com/tendermint/nomnomcoin.git
cd nomnomcoin
npm install .

node app.js --eyes="unix:///data/tendermint/data/data.sock"`)
	} else {
		var err error
		scriptBytes, err = ReadFile(app)
		if err != nil {
			return err
		}
	}

	err = WriteFile(path.Join(dir, "init.sh"), scriptBytes, 0777)
	return err
}

// Initialize common core directory
func initCoreDirectory(base string) error {
	dir := path.Join(base, "core")
	err := EnsureDir(dir, 0777)
	if err != nil {
		return err
	}

	// Write a silly sample bash script.
	scriptBytes := []byte(`#! /bin/bash
# This is a sample bash script for tendermint core
# Edit this script before "mintnet start" to change
# the core blockchain engine.

TMREPO="github.com/tendermint/tendermint"
BRANCH="master"

go get -d $TMREPO/cmd/tendermint
### DEPENDENCIES (example)
# cd $GOPATH/src/github.com/tendermint/tmsp
# git fetch origin $BRANCH
# git checkout $BRANCH
### DEPENDENCIES END
cd $GOPATH/src/$TMREPO
git fetch origin $BRANCH
git checkout $BRANCH
make install

tendermint node --seeds="$TMSEEDS" --moniker="$TMNAME" --proxy_app="$PROXYAPP"`)

	err = WriteFile(path.Join(dir, "init.sh"), scriptBytes, 0777)
	return err
}
