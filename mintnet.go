package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	. "github.com/tendermint/go-common"
	pcm "github.com/tendermint/go-process"
	client "github.com/tendermint/go-rpc/client"
	"github.com/tendermint/go-wire"
	"github.com/tendermint/netmon/types"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
	tmtypes "github.com/tendermint/tendermint/types"

	"github.com/codegangsta/cli"
)

const ValSetAnon = "anon"

var (
	machFlag = cli.StringFlag{
		Name:  "machines",
		Value: "mach[1-4]",
		Usage: "Comma separated list of machine names",
	}
)

func main() {
	app := cli.NewApp()
	app.Name = "mintnet"
	app.Usage = "mintnet [command] [args...]"
	app.Commands = []cli.Command{
		{
			Name:      "info",
			Usage:     "Information about running containers",
			ArgsUsage: "[appName]",
			Action: func(c *cli.Context) {
				cmdInfo(c)
			},
			Flags: []cli.Flag{machFlag},
			Subcommands: []cli.Command{
				{
					Name:      "port",
					Usage:     "Print container port mapping",
					ArgsUsage: "[appName]",
					Action: func(c *cli.Context) {
						cmdPorts(c)
					},
				},
			},
		},
		{
			Name:      "init",
			Usage:     "Initialize node configuration directories",
			ArgsUsage: "[baseDir]",
			Flags: []cli.Flag{
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdInit(c)
			},
			Subcommands: []cli.Command{
				{
					Name:      "chain",
					Usage:     "Initialize a new blockchain",
					ArgsUsage: "[baseDir]",
					Action: func(c *cli.Context) {
						cmdChainInit(c)
					},
					Flags: []cli.Flag{
						cli.StringFlag{
							Name:  "validator-set",
							Value: "",
							Usage: "Specify a path to the validator set for the new chain",
						},
					},
				},
				{
					Name:      "validator-set",
					Usage:     "Initialize a new validator set",
					ArgsUsage: "[baseDir]",
					Action: func(c *cli.Context) {
						cmdValidatorsInit(c)
					},
					Flags: []cli.Flag{
						cli.IntFlag{
							Name:  "N",
							Value: 4,
							Usage: "Size of the validator set",
						},
					},
				},
			},
		},
		{
			Name:      "create",
			Usage:     "Create a new Tendermint network with newly provisioned machines. Use -- to pass args through to docker-machine",
			ArgsUsage: "",
			Flags: []cli.Flag{
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdCreate(c)
			},
		},
		{
			Name:      "destroy",
			Usage:     "Destroy a Tendermint network",
			ArgsUsage: "",
			Flags: []cli.Flag{
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdDestroy(c)
			},
		},
		{
			Name:      "start",
			Usage:     "Start blockchain application",
			ArgsUsage: "[appName] [baseDir]",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "seed-machines",
					Value: "",
					Usage: "Comma separated list of machine names for seed, defaults to --machines",
				},
				cli.BoolFlag{
					Name:  "publish-all,P",
					Usage: "Publish all exposed ports to random ports",
				}, // or should we make random be default, and let users attempt to force the port?
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdStart(c)
			},
		},
		{
			Name:      "stop",
			Usage:     "Stop blockchain application",
			ArgsUsage: "[appName]",
			Flags: []cli.Flag{
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdStop(c)
			},
		},
		{
			Name:  "rm",
			Usage: "Remove blockchain application",
			Flags: []cli.Flag{
				cli.BoolFlag{
					Name:  "force",
					Usage: "Force stop app if already running",
				},
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdRm(c)
			},
		},
		{
			Name:  "docker",
			Usage: "Execute a docker command on all machines",
			Flags: []cli.Flag{
				machFlag,
			},
			Action: func(c *cli.Context) {
				cmdDocker(c)
			},
		},
	}
	app.Run(os.Args)

}

//--------------------------------------------------------------------------------
func cmdInfo(c *cli.Context) {
	cli.ShowAppHelp(c)
}

//--------------------------------------------------------------------------------

func cmdPorts(c *cli.Context) {
	args := c.Args()
	if len(args) != 1 {
		cli.ShowAppHelp(c)
		return
	}
	appName := args[0]
	machines := ParseMachines(c.GlobalString("machines"))
	for _, mach := range machines {
		portMap, err := getContainerPortMap(mach, fmt.Sprintf("%v_tmnode", appName))
		if err != nil {
			Exit(err.Error())
		}
		fmt.Println("Machine", mach)
		fmt.Println(portMap)
		fmt.Println("")
	}
}

//--------------------------------------------------------------------------------

func cmdInit(c *cli.Context) {
	cli.ShowAppHelp(c)
}

func cmdValidatorsInit(c *cli.Context) {
	args := c.Args()
	if len(args) != 1 {
		cli.ShowAppHelp(c)
		return
	}
	base := args[0]

	N := c.Int("N")
	vals := make([]*types.Validator, N)

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
		vals[i] = &types.Validator{
			ID:     name,
			PubKey: privVal.PubKey,
		}
	}

	valSet := types.ValidatorSet{
		ID:         path.Base(base),
		Validators: vals,
	}
	// write the validator set file
	b := wire.JSONBytes(valSet)

	err := ioutil.WriteFile(path.Join(base, "validator_set.json"), b, 0444)
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

	err := initDataDirectory(base)
	if err != nil {
		Exit(err.Error())
	}
	err = initAppDirectory(base)
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

		var valSet types.ValidatorSet
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
				Amount: 1,
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
		AppHash:     nil,
	}

	// Write genesis file.
	for _, mach := range machines {
		genDoc.SaveAs(path.Join(base, mach, "core", "genesis.json"))
	}

	// write the chain meta data (ie. validator set name and validators)
	blockchainCfg := &types.BlockchainConfig{
		ValSetID:   valSetID,
		Validators: make([]*types.ValidatorState, len(genVals)),
	}

	for i, v := range genVals {
		blockchainCfg.Validators[i] = &types.ValidatorState{
			Config: &types.ValidatorConfig{
				Validator: &types.Validator{ID: v.Name, PubKey: v.PubKey},
				Index:     i, // XXX: we may want more control here
			},
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
func initAppDirectory(base string) error {
	dir := path.Join(base, "app")
	err := EnsureDir(dir, 0777)
	if err != nil {
		return err
	}

	// Write a silly sample bash script.
	scriptBytes := []byte(`#! /bin/bash
# This is a sample bash script for a TMSP application

cd app/
git clone https://github.com/tendermint/nomnomcoin.git
cd nomnomcoin
npm install .

node app.js --eyes="unix:///data/tendermint/data/data.sock"`)

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

//--------------------------------------------------------------------------------

func cmdCreate(c *cli.Context) {
	args := c.Args()
	machines := ParseMachines(c.String("machines"))

	errs := provisionMachines(machines, args)
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Successfully created %v machines", len(machines)))
	}
}

func provisionMachines(machines []string, args []string) (errs []error) {
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			err := provisionMachine(args, mach)
			if err != nil {
				errs = append(errs, err)
			}
		}(mach)
	}
	wg.Wait()
	return errs
}

func provisionMachine(args []string, mach string) error {
	args = append([]string{"create"}, args...)
	args = append(args, mach)
	if !runProcess("provision-"+mach, "docker-machine", args) {
		return errors.New("Failed to provision machine " + mach)
	}
	return nil
}

//--------------------------------------------------------------------------------

func cmdDestroy(c *cli.Context) {
	machines := ParseMachines(c.String("machines"))

	// Destroy each machine.
	//var wg sync.WaitGroup
	for _, mach := range machines {
		//wg.Add(1)
		//go func(mach string) {
		//defer wg.Done()
		err := stopMachine(mach)
		if err != nil {
			fmt.Println(Red(err.Error()))
			return
		}
		err = removeMachine(mach)
		if err != nil {
			fmt.Println(Red(err.Error()))
		}
		//}(mach)
	}
	//wg.Wait()

	fmt.Println("Success!")
}

//--------------------------------------------------------------------------------

func cmdStart(c *cli.Context) {
	args := c.Args()
	if len(args) != 2 {
		cli.ShowAppHelp(c)
		return
	}
	app := args[0]
	base := args[1]
	machines := ParseMachines(c.String("machines"))
	seedMachines := ParseMachines(c.String("seed-machines"))
	if len(seedMachines) == 0 {
		seedMachines = machines
	}

	// chain config tells us which validator set we're working with (named or anon)
	chainCfg, err := ReadBlockchainConfig(base)
	if err != nil {
		Exit(err.Error())
	}
	chainCfg.ID = app

	// Get machine ips
	seeds := make([]string, len(seedMachines))
	for i, mach := range seedMachines {
		ip, err := getMachineIP(mach)
		if err != nil {
			Exit(err.Error())
		}
		// XXX: we try these by default regardless...
		// else need to update script to not pass --seeds if there are none
		// (ie. if randomPorts == true)
		seeds[i] = ip + ":46656"
	}

	// necessary if we are running multiple chains on one machine
	randomPorts := c.Bool("publish-all")

	// Initialize TMData, TMApp, and TMNode container on each machine
	// We let nodes boot and then detect which port they're listening on to collect seeds
	var wg sync.WaitGroup
	seedsCh := make(chan *types.ValidatorConfig, len(machines))
	errCh := make(chan error, len(machines))
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			if err := startTMCommon(mach, app); err != nil {
				errCh <- err
				return
			}
			if err := startTMData(mach, app); err != nil {
				errCh <- err
				return
			}
			if err := copyNodeDir(mach, app, base); err != nil {
				errCh <- err
				return
			}
			if err := startTMApp(mach, app); err != nil {
				errCh <- err
				return
			}
			seed, err := startTMNode(mach, app, seeds, randomPorts)
			if err != nil {
				errCh <- err
				return
			}
			seedsCh <- seed
		}(mach)
	}
	wg.Wait()

	var valConfs []*types.ValidatorConfig
	seeds = []string{} // for convenienve if we still need to dial them
	for i := 0; i < len(machines); i++ {
		select {
		case err = <-errCh:
			fmt.Println(Red(err.Error()))
		case valConf := <-seedsCh:
			valConfs = append(valConfs, valConf)
			seeds = append(seeds, valConf.P2PAddr)
		}
	}

	// fill in validators and write chain config to file
	for i, valCfg := range valConfs {
		valCfg.Index = chainCfg.Validators[i].Config.Index
		chainCfg.Validators[i] = &types.ValidatorState{
			Config: valCfg,
		}

	}
	if err := WriteBlockchainConfig(base, chainCfg); err != nil {
		fmt.Println(string(wire.JSONBytes(chainCfg)))
		Exit(err.Error())
	}

	// bail if anything failed; we've already written anyone that didnt to file
	if err != nil {
		Exit(err.Error())
	}

	if randomPorts {
		// dial the seeds
		fmt.Println(Green("Instruct nodes to dial eachother"))
		for _, val := range valConfs {
			wg.Add(1)
			go func(rpcAddr string) {
				defer wg.Done()
				if err := dialSeeds(rpcAddr, seeds); err != nil {
					fmt.Println(Red(err.Error()))
					return
				}
			}(val.RPCAddr)
		}
		wg.Wait()
	}
	fmt.Println(Green("Done launching tendermint network for " + app))
}

func ReadBlockchainConfig(base string) (*types.BlockchainConfig, error) {
	chainCfg := new(types.BlockchainConfig)
	err := ReadJSONFile(chainCfg, path.Join(base, "chain_config.json"))
	return chainCfg, err
}

func WriteBlockchainConfig(base string, chainCfg *types.BlockchainConfig) error {
	b := wire.JSONBytes(chainCfg)
	var buf bytes.Buffer
	json.Indent(&buf, b, "", "\t")
	return ioutil.WriteFile(path.Join(base, "chain_config.json"), buf.Bytes(), 0600)
}

/*
func listMachinesFromBase(base string) ([]string, error) {
	files, err := ioutil.ReadDir(base)
	if err != nil {
		return nil, err
	}
	machines := []string{}
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		if file.Name() == "app" {
			continue
		}
		machines = append(machines, file.Name())
	}
	return machines, nil
}
*/

func startTMCommon(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmcommon --entrypoint /bin/echo tendermint/tmbase`, app)}
	if !runProcess("start-tmcommon-"+mach, "docker-machine", args) {
		return errors.New("Failed to start tmcommon on machine " + mach)
	}
	return nil
}

func copyNodeDir(mach, app, base string) error {
	err := copyToMachine(mach, app, path.Join(base, "data"), "/data/tendermint/data", true)
	if err != nil {
		return err
	}
	err = copyToMachine(mach, app, path.Join(base, "app"), "/data/tendermint/app", true)
	if err != nil {
		return err
	}
	err = copyToMachine(mach, app, path.Join(base, "core"), "/data/tendermint/core", true)
	if err != nil {
		return err
	}
	err = copyToMachine(mach, app, path.Join(base, mach, "core", "/."), "/data/tendermint/core", true)
	if err != nil {
		return err
	}
	return nil
}

// Starts data service and checks for existence of /data/tendermint/data/data.sock
func startTMData(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmdata --volumes-from %v_tmcommon -d `+
		`tendermint/tmbase /data/tendermint/data/init.sh`, app, app)}
	if !runProcess("start-tmdata-"+mach, "docker-machine", args) {
		return errors.New("Failed to start tmdata on machine " + mach)
	}
	for i := 1; i < 10; i++ { // TODO configure
		time.Sleep(time.Duration(i) * time.Second)
		if checkFileExists(mach, app+"_tmdata", "/data/tendermint/data/data.sock") {
			return nil
		}
	}
	return errors.New("Failed to start tmdata on machine " + mach + " (timeout)")
}

func startTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmapp --volumes-from %v_tmcommon -d `+
		`tendermint/tmbase /data/tendermint/app/init.sh`, app, app)}
	if !runProcess("start-tmapp-"+mach, "docker-machine", args) {
		return errors.New("Failed to start tmapp on machine " + mach)
	}
	return nil
}

func startTMNode(mach, app string, seeds []string, randomPort bool) (*types.ValidatorConfig, error) {
	portString := "-p 46656:46656 -p 46657:46657"
	if randomPort {
		portString = "-P"
	}

	proxyApp := Fmt("tcp://%v_tmapp:46658", app)
	tmRoot := "/data/tendermint/core"
	args := []string{"ssh", mach, Fmt(`docker run %v --name %v_tmnode --volumes-from %v_tmcommon -d `+
		`--link %v_tmapp -e TMNAME="%v" -e TMSEEDS="%v" -e TMROOT="%v" -e PROXYAPP="%v" `+
		`tendermint/tmbase /data/tendermint/core/init.sh`,
		portString, app, app, app, eB(mach), eB(strings.Join(seeds, ",")), tmRoot, eB(proxyApp))}
	if !runProcess("start-tmnode-"+mach, "docker-machine", args) {
		return nil, errors.New("Failed to start tmnode on machine " + mach)
	}

	// Give it some time to install and make repo.
	time.Sleep(time.Second * 10)

	// Get the node's validator info
	// Need to retry to wait until tendermint is installed
	for {
		args = []string{"ssh", mach, Fmt(`docker exec %v_tmnode tendermint show_validator --log_level=error`, app)}
		output, ok := runProcessGetResult("show-validator-tmnode-"+mach, "docker-machine", args)
		if !ok || output == "" {
			fmt.Println(Yellow(Fmt("tendermint not yet installed in %v. Waiting...", mach)))
			time.Sleep(time.Second * 5)
			continue
		} else {
			fmt.Println(Fmt("validator for %v: %v", mach, output))

			// now grab the node's public address and port
			ip, err := getMachineIP(mach)
			if err != nil {
				return nil, err
			}

			valConfig := &types.ValidatorConfig{
				Validator: &types.Validator{
					ID: mach,
				},
			}

			var p2pPort, rpcPort = "46656", "46657"
			if randomPort {
				portMap, err := getContainerPortMap(mach, fmt.Sprintf("%v_tmnode", app))
				if err != nil {
					return nil, err
				}
				p2pPort, ok = portMap["46656"]
				if !ok {
					return nil, errors.New("No port map found for p2p port 46656 on mach " + mach)
				}
				rpcPort, ok = portMap["46657"]
				if !ok {
					return nil, errors.New("No port map found for rpc port 46657 on mach " + mach)
				}
			}
			valConfig.P2PAddr = fmt.Sprintf("%v:%v", ip, p2pPort)
			valConfig.RPCAddr = fmt.Sprintf("%v:%v", ip, rpcPort)

			// get pubkey from rpc endpoint
			// try a few times in case the rpc server is slow to start
			var result ctypes.TMResult
			for i := 0; i < 5; i++ {
				time.Sleep(time.Second)
				c := client.NewClientURI(fmt.Sprintf("http://%s", valConfig.RPCAddr))
				if _, err = c.Call("status", nil, &result); err != nil {
					continue
				}
				status := result.(*ctypes.ResultStatus)
				valConfig.Validator.PubKey = status.PubKey
				break
			}
			if err != nil {
				return nil, fmt.Errorf("Error getting PubKey from mach %s on %s: %v", mach, valConfig.RPCAddr, err)
			}

			return valConfig, nil
		}
	}
	return nil, nil
}

func dialSeeds(rpcAddr string, seeds []string) error {
	var result ctypes.TMResult
	c := client.NewClientURI(fmt.Sprintf("http://%s", rpcAddr))
	args := map[string]interface{}{"seeds": seeds}
	if _, err := c.Call("dial_seeds", args, &result); err != nil {
		return errors.New("Error dialing seeds at rpc address " + rpcAddr)
	}
	return nil
}

func getContainerPortMap(mach, container string) (map[string]string, error) {
	args := []string{"ssh", mach, Fmt(`docker port %v`, container)}
	output, ok := runProcessGetResult(fmt.Sprintf("get-ports-%v-%v", mach, container), "docker-machine", args)
	if !ok {
		return nil, errors.New("Failed to get the exposed ports on machine " + mach + " for container " + container)
	}
	// what a hack. might be time to start using the go-dockerclient or eris-cli packages
	portMap := make(map[string]string)
	spl := strings.Split(string(output), "\n")
	for _, s := range spl {
		// 4001/tcp -> 0.0.0.0:32769
		spl2 := strings.Split(s, "->")
		if len(spl2) < 2 {
			continue
		}
		port := strings.TrimSpace(strings.Split(spl2[0], "/")[0])
		mapS := strings.Split(spl2[1], ":")
		mappedTo := strings.TrimSpace(mapS[len(mapS)-1])
		portMap[port] = mappedTo
	}
	return portMap, nil
}

//--------------------------------------------------------------------------------

func cmdStop(c *cli.Context) {
	args := c.Args()
	if len(args) == 0 {
		Exit("stop requires argument for app name")
	}
	app := args[0]
	machines := ParseMachines(c.String("machines"))

	// Initialize TMCommon, TMData, TMApp, and TMNode container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			stopTMNode(mach, app)
			stopTMApp(mach, app)
		}(mach)
	}
	wg.Wait()
}

func stopTMNode(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker stop %v_tmnode`, app)}
	if !runProcess("stop-tmnode-"+mach, "docker-machine", args) {
		return errors.New("Failed to stop tmnode on machine " + mach)
	}
	return nil
}

func stopTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker stop %v_tmapp`, app)}
	if !runProcess("stop-tmapp-"+mach, "docker-machine", args) {
		return errors.New("Failed to stop tmapp on machine " + mach)
	}
	return nil
}

//--------------------------------------------------------------------------------

func cmdRm(c *cli.Context) {
	args := c.Args()
	if len(args) == 0 {
		Exit("rm requires argument for app name")
	}
	app := args[0]
	machines := ParseMachines(c.String("machines"))
	force := c.Bool("force")

	if force {
		// Stop TMNode/TMApp if running
		var wg sync.WaitGroup
		for _, mach := range machines {
			wg.Add(1)
			go func(mach string) {
				defer wg.Done()
				stopTMNode(mach, app)
				stopTMApp(mach, app)
			}(mach)
		}
		wg.Wait()
	}

	// Initialize TMCommon, TMApp, and TMNode container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			rmTMCommon(mach, app)
			rmTMApp(mach, app)
			rmTMNode(mach, app)
		}(mach)
	}
	wg.Wait()
}

func rmTMCommon(mach, app string) error {
	// XXX: "-v" is clutch for dev. without it, volumes build up on disk.
	// would be great if we had flags that pass through mintnet to docker
	// but this is somewhat complicated by fact we are managing three or four containers with one command
	args := []string{"ssh", mach, Fmt(`docker rm -v %v_tmcommon`, app)}
	if !runProcess("rm-tmcommon-"+mach, "docker-machine", args) {
		return errors.New("Failed to rm tmcommon on machine " + mach)
	}
	return nil
}

func rmTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker rm -v %v_tmapp`, app)}
	if !runProcess("rm-tmapp-"+mach, "docker-machine", args) {
		return errors.New("Failed to rm tmapp on machine " + mach)
	}
	return nil
}

func rmTMNode(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker rm -v %v_tmnode`, app)}
	if !runProcess("rm-tmnode-"+mach, "docker-machine", args) {
		return errors.New("Failed to rm tmnode on machine " + mach)
	}
	return nil
}

//--------------------------------------------------------------------------------

func cmdDocker(c *cli.Context) {
	args := c.Args()
	machines := ParseMachines(c.String("machines"))

	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			dockerCmd(mach, args)
		}(mach)
	}
	wg.Wait()
}

func dockerCmd(mach string, args []string) error {
	args = []string{"ssh", mach, "docker " + strings.Join(args, " ")}
	if !runProcess("docker-cmd-"+mach, "docker-machine", args) {
		return errors.New("Failed to exec docker command on machine " + mach)
	}
	return nil
}

//--------------------------------------------------------------------------------

// Stop a machine
// mach: name of machine
func stopMachine(mach string) error {
	args := []string{"stop", mach}
	if !runProcess("stop-"+mach, "docker-machine", args) {
		return errors.New("Failed to stop machine " + mach)
	}
	return nil
}

// Remove a machine
// mach: name of machine
func removeMachine(mach string) error {
	args := []string{"rm", mach}
	if !runProcess("remove-"+mach, "docker-machine", args) {
		return errors.New("Failed to remove machine " + mach)
	}
	return nil
}

// List machine names that match prefix
func listMachines(prefix string) ([]string, error) {
	args := []string{"ls", "--quiet"}
	output, ok := runProcessGetResult("list-machines", "docker-machine", args)
	if !ok {
		return nil, errors.New("Failed to list machines")
	}
	output = strings.TrimSpace(output)
	if len(output) == 0 {
		return nil, nil
	}
	machines := strings.Split(output, "\n")
	matched := []string{}
	for _, mach := range machines {
		if strings.HasPrefix(mach, prefix+"-") {
			matched = append(matched, mach)
		}
	}
	return matched, nil
}

// Get ip of a machine
// mach: name of machine
func getMachineIP(mach string) (string, error) {
	args := []string{"ip", mach}
	output, ok := runProcessGetResult("get-ip-"+mach, "docker-machine", args)
	if !ok {
		return "", errors.New("Failed to get ip of machine" + mach)
	}
	return strings.TrimSpace(output), nil
}

// Copy a file (or dir recursively) from srcPath (local machine) to
// dstPath in the tmnode container.
func copyToMachine(mach string, app string, srcPath string, dstPath string, copyContents bool) error {

	// First, copy the file to a temporary location
	// in the machine.
	tempFile := "temp_" + RandStr(12)
	args := []string{"scp", "-r", srcPath, mach + ":" + tempFile}
	if !runProcess("scp-file-"+mach, "docker-machine", args) {
		return errors.New("Failed to copy file to machine " + mach)
	}

	// Next, docker cp the file into the container
	if copyContents {
		tempFile = tempFile + "/."
	}
	args = []string{"ssh", mach, Fmt("docker cp %v %v_tmcommon:%v", tempFile, app, dstPath)}
	if !runProcess("docker-cp-file-"+mach, "docker-machine", args) {
		return errors.New("Failed to docker-cp file to container in machine " + mach)
	}

	// Next, change the ownership of the file to tmuser
	// TODO We don't really want to change all the permissions
	args = []string{"ssh", mach, Fmt(`docker run --rm --volumes-from %v_tmcommon -u root tendermint/tmbase chown -R tmuser:tmuser %v`, app, dstPath)}
	if !runProcess("docker-chmod-file-"+mach, "docker-machine", args) {
		return errors.New("Failed to docker-run(chmod) file in machine " + mach)
	}

	// TODO: remove tempFile
	return nil
}

// NOTE: returns false if any error
func checkFileExists(mach string, container string, path string) bool {
	args := []string{"ssh", mach, Fmt(`docker exec %v ls %v`, container, path)}
	_, ok := runProcessGetResult("check-file-exists-"+mach, "docker-machine", args)
	return ok
}

//--------------------------------------------------------------------------------

func runProcess(label string, command string, args []string) bool {
	outFile := NewBufferCloser(nil)
	proc, err := pcm.StartProcess(label, command, args, nil, outFile)
	if err != nil {
		fmt.Println(Red(err.Error()))
		return false
	}

	<-proc.WaitCh
	fmt.Println(Green(command), Green(args))
	if proc.ExitState.Success() {
		fmt.Println(Blue(string(outFile.Bytes())))
		return true
	} else {
		// Error!
		fmt.Println(Red(string(outFile.Bytes())))
		return false
	}
}

func runProcessGetResult(label string, command string, args []string) (string, bool) {
	outFile := NewBufferCloser(nil)
	proc, err := pcm.StartProcess(label, command, args, nil, outFile)
	if err != nil {
		return "", false
	}

	<-proc.WaitCh
	fmt.Println(Green(command), Green(args))
	if proc.ExitState.Success() {
		fmt.Println(Blue(string(outFile.Bytes())))
		return string(outFile.Bytes()), true
	} else {
		// Error!
		fmt.Println(Red(string(outFile.Bytes())))
		return string(outFile.Bytes()), false
	}
}

//--------------------------------------------------------------------------------

func eB(s string) string {
	s = strings.Replace(s, `\`, `\\`, -1)
	s = strings.Replace(s, `$`, `\$`, -1)
	s = strings.Replace(s, `"`, `\"`, -1)
	s = strings.Replace(s, `'`, `\'`, -1)
	s = strings.Replace(s, `!`, `\!`, -1)
	s = strings.Replace(s, `#`, `\#`, -1)
	s = strings.Replace(s, `%`, `\%`, -1)
	s = strings.Replace(s, "\t", `\t`, -1)
	s = strings.Replace(s, "`", "\\`", -1)
	return s
}

func condenseBash(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	lines := strings.Split(cmd, "\n")
	res := []string{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		res = append(res, line)
	}
	return strings.Join(res, "; ")
}

//--------------------------------------------------------------------------------

func ReadJSONFile(o interface{}, filename string) error {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}
	wire.ReadJSON(o, b, &err)
	if err != nil {
		return err
	}
	return nil
}
