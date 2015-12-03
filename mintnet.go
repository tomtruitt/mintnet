package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	. "github.com/tendermint/go-common"
	"github.com/tendermint/go-crypto"
	pcm "github.com/tendermint/go-process"
	"github.com/tendermint/go-wire"
	"github.com/tendermint/tendermint/types"

	"github.com/codegangsta/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "mintnet"
	app.Usage = "mintnet [command] [args...]"
	app.Commands = []cli.Command{
		{
			Name:  "init",
			Usage: "Initialize node configuration directories",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "nodes",
					Value: 4,
					Usage: "number of nodes",
				},
				cli.StringFlag{
					Name:  "prefix",
					Value: "node",
					Usage: "node name prefix",
				},
			},
			Action: func(c *cli.Context) {
				cmdInit(c)
			},
		},
		{
			Name:  "create",
			Usage: "Create a new Tendermint network with newly provisioned machines",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "nodes",
					Value: 4,
					Usage: "number of nodes",
				},
				cli.StringFlag{
					Name:  "prefix",
					Value: "testnode",
					Usage: "node name prefix",
				},
				cli.StringFlag{
					Name:  "tmrepo",
					Value: "github.com/tendermint/tendermint",
					Usage: "tm repository to pull",
				},
				cli.StringFlag{
					Name:  "tmhead",
					Value: "origin/master",
					Usage: "tm branch/commit-hash to make & run",
				},
				cli.StringFlag{
					Name:  "apprepo",
					Value: "github.com/tendermint/tmsp",
					Usage: "app repository to pull",
				},
				cli.StringFlag{
					Name:  "apphead",
					Value: "origin/master",
					Usage: "app branch/commit-hash to make & run",
				},
				cli.StringFlag{
					Name:  "gen-file-in",
					Value: "empty-genesis.json",
					Usage: "input genesis file for reading accounts, etc",
				},
				cli.StringFlag{
					Name:  "gen-file-out",
					Value: "genesis.json",
					Usage: "output genesis file with new validators",
				},
			},
			Action: func(c *cli.Context) {
				cmdCreate(c)
			},
		},
		{
			Name:  "copy-genesis",
			Usage: "Copy genesis file to all nodes",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "nodes",
					Value: 4,
					Usage: "number of nodes",
				},
				cli.StringFlag{
					Name:  "prefix",
					Value: "testnode",
					Usage: "node name prefix",
				},
				cli.StringFlag{
					Name:  "gen-file",
					Value: "genesis.json",
					Usage: "genesis file to copy",
				},
			},
			Action: func(c *cli.Context) {
				cmdCopyGenesis(c)
			},
		},
		{
			Name:  "destroy",
			Usage: "Destroy a Tendermint network",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "prefix",
					Value: "testnode",
					Usage: "node name prefix",
				},
			},
			Action: func(c *cli.Context) {
				cmdDestroy(c)
			},
		},
	}
	app.Run(os.Args)

}

//--------------------------------------------------------------------------------

// Initialize directories for each node
func cmdInit(c *cli.Context) {
	args := c.Args()
	base := ""
	if len(args) > 0 {
		base = args[0]
	}
	nodes := c.Int("nodes")
	prefix := c.String("prefix")

	err := initAppDirectory(base)
	if err != nil {
		Exit(err.Error())
	}

	genVals := make([]types.GenesisValidator, nodes)

	// Initialize core dir and priv_validator.json's
	for i := 0; i < nodes; i++ {
		name := Fmt("%v-%v", prefix, i)
		err := initCoreDirectory(base, name)
		if err != nil {
			Exit(err.Error())
		}
		// Read priv_validator.json to populate genVals
		privValFile := path.Join(base, name, "core", "priv_validator.json")
		privVal := types.LoadPrivValidator(privValFile)
		genVals[i] = types.GenesisValidator{
			PubKey: privVal.PubKey,
			Amount: 1,
			Name:   name,
		}
	}

	// Generate genesis doc from generated validators
	genDoc := &types.GenesisDoc{
		GenesisTime: time.Now(),
		ChainID:     "chain-" + RandStr(6),
		Validators:  genVals,
		AppHash:     nil,
	}

	// Write genesis file.
	for i := 0; i < nodes; i++ {
		name := Fmt("%v-%v", prefix, i)
		genDoc.SaveAs(path.Join(base, name, "genesis.json"))
	}

}

func initCoreDirectory(base, name string) error {
	dir := path.Join(base, name, "core")
	err := EnsureDir(dir)
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
	privValidator := types.GenPrivValidator()
	privValidator.SetFile(file)
	privValidator.Save()
}

func initAppDirectory(base string) error {
	dir := path.Join(base, "app")
	err := EnsureDir(dir)
	if err != nil {
		return err
	}

	// Write a silly sample bash script.
	scriptBytes := []byte(`#! /bin/bash
# This is a sample bash script for a TMSP application
# The source code for this sample application is XXX
# Edit this script before "mintnet deploy" to change
# the application being run.
# NOTE: This script is tailored for a Go-based project.
# Want other languages?  Let us know!  support@tendermint.com

REPO =    "github.com/tendermint/tmsp"
HEAD =    "origin/master"
CMD  =    "counter"

mkdir -p $GOPATH/src/$REPO
cd $GOPATH/src/$REPO
git clone https://$REPO.git .
git fetch
git reset --hard $HEAD
go get -d $REPO/cmd/$CMD
CMD --address="tcp://0.0.0.0:46658"`)

	err = WriteFile(path.Join(dir, "init.sh"), scriptBytes)
	return err
}

// Create a new Tendermint network with newly provisioned machines
func cmdCreate(c *cli.Context) {
	args := c.Args()             // Args to docker-machine
	prefix := c.String("prefix") // Machine name prefix
	numNodes := c.Int("nodes")

	// Provision numNodes machines
	errs := provisionMachines(prefix, numNodes, args)
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Successfully deployed %v machines", numNodes))
	}

	// Get machine ips
	ips, errs := getIPMachines(prefix, numNodes)
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Machine ips: %v", ips))
	}

	// Generate seeds from those ips
	seeds := ""
	for i, ip := range ips {
		if i > 0 {
			seeds = seeds + ","
		}
		seeds = seeds + ip + ":46656"
	}

	// Run containers.
	// Pull repo to given head.
	tmrepo := c.String("tmrepo")
	tmhead := c.String("tmhead")
	apprepo := c.String("apprepo")
	apphead := c.String("apphead")
	infos, errs := initMachines(prefix, numNodes, tmrepo, tmhead, apprepo, apphead, seeds)
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Successfully initialized %v machines", numNodes))
	}
	fmt.Println("Infos", infos)

	// Read input genesis
	genInFile, genOutFile := c.String("gen-file-in"), c.String("gen-file-out")
	genInBytes, err := ioutil.ReadFile(genInFile)
	if err != nil {
		Exit(Fmt("Couldn't read input genesis file: %v", err))
	}
	genDoc := types.GenesisDocFromJSON(genInBytes)

	// Replace validators
	genDoc.Validators = nil
	for _, info := range infos {
		genDoc.Validators = append(genDoc.Validators, types.GenesisValidator{
			Name:   info.Name,
			PubKey: info.PubKey,
			Amount: 1,
		})
	}

	// Replace other info
	genDoc.GenesisTime = time.Now()
	genDoc.ChainID = Fmt("testnet-%v", RandStr(6))

	// Write output genesis
	genOutBytes := wire.JSONBytesPretty(genDoc)
	MustWriteFile(genOutFile, genOutBytes)
	fmt.Println(string(genOutBytes))
	fmt.Println(Fmt("Wrote output genesis to %v", genOutFile))
}

// Destroy a Tendermint network
// Stops and removes machines from docker-machine.
func cmdDestroy(c *cli.Context) {
	prefix := c.String("prefix")

	// First, get machines
	machines, err := listMachines(prefix)
	if err != nil {
		Exit(Fmt("Failed to list machines: %v", err))
	}

	// Destroy each machine.
	var wg sync.WaitGroup
	for _, name := range machines {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			err := stopMachine(name)
			if err != nil {
				fmt.Println(Red(err.Error()))
				return
			}
			err = removeMachine(name)
			if err != nil {
				fmt.Println(Red(err.Error()))
			}
		}(name)
	}
	wg.Wait()

	fmt.Println("Success!")
}

// Copy genesis file to network, launching it.
func cmdCopyGenesis(c *cli.Context) {
	prefix := c.String("prefix")
	numNodes := c.Int("nodes")
	genFile := c.String("gen-file")

	// Copy output genesis to machines
	errs := copyFileToMachines(prefix, numNodes, genFile, "/data/tendermint/genesis.json")
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Successfully copied genesis to %v machines", numNodes))
	}
}

//--------------------------------------------------------------------------------

// Provision a number of machines using docker-machine.
// prefix: node name prefix
// numNodes: number of nodes to provision
// args: arguments to docker-machine
func provisionMachines(prefix string, numNodes int, args []string) (errs []error) {
	var wg sync.WaitGroup
	for i := 1; i <= numNodes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			err := provisionMachine(args, Fmt("%v-%v", prefix, i))
			if err != nil {
				errs = append(errs, err)
			}
		}(i)
	}
	wg.Wait()

	return errs
}

// Provision a new machine using docker-machine.
// args: arguments to docker-machine
// name: name of machine
func provisionMachine(args []string, name string) error {
	args = append([]string{"create"}, args...)
	args = append(args, name)
	if !runProcess("provision-"+name, "docker-machine", args) {
		return errors.New("Failed to provision machine " + name)
	}
	return nil
}

// Initialize a number of machines using docker-machine.
// prefix: node name prefix
// numNodes: number of nodes to init
// tmrepo: tm repository to pull from, e.g. github.com/tendermint/tendermint
// tmhead: git commit hash to make and run
// seeds: seed list
func initMachines(prefix string, numNodes int, tmrepo string, tmhead string, apprepo string, apphead string, seeds string) (infos []MachineInfo, errs []error) {
	var wg sync.WaitGroup
	for i := 1; i <= numNodes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := Fmt("%v-%v", prefix, i)
			pubKeyStr, err := initMachine(name, tmrepo, tmhead, apprepo, apphead, seeds)
			if err != nil {
				errs = append(errs, err)
				return
			}
			pubKey, err := readPubKeyEd25519(pubKeyStr)
			if err != nil {
				errs = append(errs, err)
				return
			}
			infos = append(infos, MachineInfo{
				Name:   name,
				PubKey: pubKey,
			})
		}(i)
	}
	wg.Wait()

	return infos, errs
}

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

// Initialize a new machine using docker-machine.
// name: name of machine
// tmrepo: tm repository to pull from, e.g. github.com/tendermint/tendermint
// tmhead: git commit hash to make and run
// apprepo: app repository to pull from, e.g. github.com/tendermint/tmsp
// apphead: git commit hash to make and run
// seeds: seed list
func initMachine(name string, tmrepo string, tmhead string, apprepo string, apphead string, seeds string) (info string, err error) {
	// Initialize the tmdata container
	args := []string{"ssh", name, "docker run --name tmdata --entrypoint /bin/echo tendermint/tmbase Data-only container for node"}
	if !runProcess("init-tmdata-"+name, "docker-machine", args) {
		return "", errors.New("Failed to init tmdata on machine " + name)
	}

	// Initialize the tmapp container
	// XXX make this more general.
	runScript := condenseBash(`
mkdir -p $GOPATH/src/$REPO
cd $GOPATH/src/$REPO
git clone https://$REPO.git .
git fetch
git reset --hard $HEAD
go get -d $REPO/cmd/counter
go run cmd/counter/main.go --address="tcp://0.0.0.0:46658"
`)
	// XXX do not expose 46658 to the public.
	args = []string{"ssh", name, Fmt(`docker run --name tmapp --volumes-from tmdata -d -p 46658:46658 `+
		`-e REPO="%v" -e HEAD="%v" tendermint/tmbase /bin/bash -c "%v"`,
		eB(apprepo), eB(apphead), eB(runScript))}
	fmt.Println(args)
	if !runProcess("init-tmapp-"+name, "docker-machine", args) {
		return "", errors.New("Failed to init tmapp on machine " + name)
	}

	// Initialize the tmnode container
	runScript = condenseBash(`
mkdir -p $GOPATH/src/$TMREPO
cd $GOPATH/src/$TMREPO
git clone https://$TMREPO.git .
git fetch
git reset --hard $TMHEAD
go get -d $TMREPO/cmd/tendermint
make
tendermint node --seeds="$TMSEEDS" --moniker="$TMNAME"
`)
	args = []string{"ssh", name, Fmt(`docker run --name tmnode --volumes-from tmdata -d -p 46656:46656 -p 46657:46657 `+
		`-e TMNAME="%v" -e TMREPO="%v" -e TMHEAD="%v" -e TMSEEDS="%v" tendermint/tmbase /bin/bash -c "%v"`,
		eB(name), eB(tmrepo), eB(tmhead), eB(seeds), eB(runScript))}
	fmt.Println(args)
	if !runProcess("init-tmnode-"+name, "docker-machine", args) {
		return "", errors.New("Failed to init tmnode on machine " + name)
	}

	// Give it some time to install and make repo.
	time.Sleep(time.Second * 10)

	// Get the node's validator info
	// Need to retry to wait until tendermint is installed
	for {
		args = []string{"ssh", name, Fmt("docker exec tmnode tendermint show_validator --log_level=error")}
		output, ok := runProcessGetResult("show-validator-tmnode-"+name, "docker-machine", args)
		if !ok || output == "" {
			fmt.Println(Yellow(Fmt("tendermint not yet installed in %v. Waiting...", name)))
			time.Sleep(time.Second * 5)
			continue
			// return "", errors.New("Failed to get tmnode validator on machine " + name)
		} else {
			fmt.Println(Fmt("validator for %v: %v", name, output))
			return output, nil
		}
	}
}

// Stop a machine
// name: name of machine
func stopMachine(name string) error {
	args := []string{"stop", name}
	if !runProcess("stop-"+name, "docker-machine", args) {
		return errors.New("Failed to stop machine " + name)
	}
	return nil
}

// Remove a machine
// name: name of machine
func removeMachine(name string) error {
	args := []string{"rm", name}
	if !runProcess("remove-"+name, "docker-machine", args) {
		return errors.New("Failed to remove machine " + name)
	}
	return nil
}

// List machine names
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
	for _, name := range machines {
		if strings.HasPrefix(name, prefix+"-") {
			matched = append(matched, name)
		}
	}
	return matched, nil
}

// Get ip of a machines
func getIPMachines(prefix string, numNodes int) (ips []string, errs []error) {
	for i := 1; i <= numNodes; i++ {
		name := Fmt("%v-%v", prefix, i)
		ip, err := getIPMachine(name)
		if err != nil {
			errs = append(errs, err)
		} else {
			ips = append(ips, ip)
		}
	}
	return ips, errs
}

// Get ip of a machine
// name: name of machine
func getIPMachine(name string) (string, error) {
	args := []string{"ip", name}
	output, ok := runProcessGetResult("get-ip-"+name, "docker-machine", args)
	if !ok {
		return "", errors.New("Failed to get ip of machine" + name)
	}
	return strings.TrimSpace(output), nil
}

func copyFileToMachines(prefix string, numNodes int, srcPath string, dstPath string) (errs []error) {
	var wg sync.WaitGroup
	for i := 1; i <= numNodes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := Fmt("%v-%v", prefix, i)
			err := copyToMachine(name, srcPath, dstPath)
			if err != nil {
				errs = append(errs, err)
				return
			}
		}(i)
	}
	wg.Wait()

	return errs
}

// Copy a file from srcPath (local machine) to
// dstPath in the tmnode container.
func copyToMachine(name string, srcPath string, dstPath string) error {

	// First, copy the file to a temporary location
	// in the machine.
	tempFile := "temp_" + RandStr(12)
	args := []string{"scp", srcPath, name + ":" + tempFile}
	if !runProcess("scp-file-"+name, "docker-machine", args) {
		return errors.New("Failed to copy file to machine " + name)
	}

	// Next, docker cp the file into the container
	args = []string{"ssh", name, Fmt("docker cp %v tmnode:%v", tempFile, dstPath)}
	if !runProcess("docker-cp-file-"+name, "docker-machine", args) {
		return errors.New("Failed to docker-cp file to container in machine " + name)
	}

	// TODO: remove tempFile

	return nil
}

//--------------------------------------------------------------------------------

func runProcess(label string, command string, args []string) bool {
	outFile := NewBufferCloser(nil)
	fmt.Println(Green(command), Green(args))
	proc, err := pcm.StartProcess(label, command, args, nil, outFile)
	if err != nil {
		fmt.Println(Red(err.Error()))
		return false
	}

	<-proc.WaitCh
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
	fmt.Println(Green(command), Green(args))
	proc, err := pcm.StartProcess(label, command, args, nil, outFile)
	if err != nil {
		return "", false
	}

	<-proc.WaitCh
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

type MachineInfo struct {
	Name   string
	PubKey crypto.PubKeyEd25519
}

func readPubKeyEd25519(str string) (pubKey crypto.PubKeyEd25519, err error) {
	wire.ReadJSONPtr(&pubKey, []byte(str), &err)
	return
}
