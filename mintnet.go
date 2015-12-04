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
	pcm "github.com/tendermint/go-process"
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
				cli.StringFlag{
					Name:  "machines",
					Value: "mach1,mach2,mach3,mach4",
					Usage: "Comma separated list of machine names",
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
				cli.StringFlag{
					Name:  "machines",
					Value: "mach1,mach2,mach3,mach4",
					Usage: "Comma separated list of machine names",
				},
			},
			Action: func(c *cli.Context) {
				cmdCreate(c)
			},
		},
		{
			Name:  "destroy",
			Usage: "Destroy a Tendermint network",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "machines",
					Value: "mach1,mach2,mach3,mach4",
					Usage: "Comma separated list of machine names",
				},
			},
			Action: func(c *cli.Context) {
				cmdDestroy(c)
			},
		},
		{
			Name:  "start",
			Usage: "Start blockchain application",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "app",
					Value: "testapp",
					Usage: "Application name",
				},
			},
			Action: func(c *cli.Context) {
				cmdStart(c)
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
	machines := strings.Split(c.String("machines"), ",")

	err := initAppDirectory(base)
	if err != nil {
		Exit(err.Error())
	}

	genVals := make([]types.GenesisValidator, len(machines))

	// Initialize core dir and priv_validator.json's
	for i, mach := range machines {
		err := initCoreDirectory(base, mach)
		if err != nil {
			Exit(err.Error())
		}
		// Read priv_validator.json to populate genVals
		privValFile := path.Join(base, mach, "core", "priv_validator.json")
		privVal := types.LoadPrivValidator(privValFile)
		genVals[i] = types.GenesisValidator{
			PubKey: privVal.PubKey,
			Amount: 1,
			Name:   mach,
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
	for _, mach := range machines {
		genDoc.SaveAs(path.Join(base, mach, "genesis.json"))
	}

	fmt.Println(Fmt("Successfully initialized %v node directories", len(machines)))
}

func initCoreDirectory(base, mach string) error {
	dir := path.Join(base, mach, "core")
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

//--------------------------------------------------------------------------------

func cmdCreate(c *cli.Context) {
	args := c.Args()
	machines := strings.Split(c.String("machines"), ",")

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
		go func() {
			defer wg.Done()
			err := provisionMachine(args, mach)
			if err != nil {
				errs = append(errs, err)
			}
		}()
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
	machines := strings.Split(c.String("machines"), ",")

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
	base := ""
	if len(args) > 0 {
		base = args[0]
	}
	app := c.String("app") // app name (prefix)
	machines, err := listMachinesFromBase(base)
	if err != nil {
		Exit(err.Error())
	}

	// Get machine ips
	seeds := make([]string, len(machines))
	for i, mach := range machines {
		ip, err := getMachineIP(mach)
		if err != nil {
			Exit(err.Error())
		}
		seeds[i] = ip + ":46656"
	}

	// Initialize TMData, TMApp, and TMNode container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			startTMData(mach, app)
			copyNodeDir(mach, app, base)
			startTMApp(mach, app)
			startTMNode(mach, app, seeds)
		}(mach)
	}
	wg.Wait()
}

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

func startTMData(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmdata --entrypoint /bin/echo tendermint/tmbase`, app)}
	if !runProcess("init-tmdata-"+mach, "docker-machine", args) {
		return errors.New("Failed to init tmdata on machine " + mach)
	}
	return nil
}

func copyNodeDir(mach, app, base string) error {
	err := copyToMachine(mach, app, path.Join(base, mach, "core"), "/data/tendermint/core")
	if err != nil {
		return err
	}
	err = copyToMachine(mach, app, path.Join(base, "app"), "/data/tendermint/app")
	if err != nil {
		return err
	}
	return nil
}

func startTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmapp --volumes-from %v_tmdata -d `+
		`tendermint/tmbase /data/tendermint/app/init.sh`, app, app)}
	fmt.Println(args)
	if !runProcess("init-tmapp-"+mach, "docker-machine", args) {
		return errors.New("Failed to init tmapp on machine " + mach)
	}
	return nil
}

func startTMNode(mach, app string, seeds []string) error {
	tmrepo := "github.com/tendermint/tendermint"
	tmhead := "origin/mintdb"
	runScript := condenseBash(`
mkdir -p $GOPATH/src/$TMREPO
cd $GOPATH/src/$TMREPO
git clone https://$TMREPO.git .
git fetch
git reset --hard $TMHEAD
go get -d $TMREPO/cmd/tendermint
make
tendermint node --seeds="$TMSEEDS" --moniker="$TMNAME"
`)
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmnode --volumes-from %v_tmdata -d --link %v_tmapp -p 46656:46656 -p 46657:46657 `+
		`-e TMNAME="%v" -e TMREPO="%v" -e TMHEAD="%v" -e TMSEEDS="%v" tendermint/tmbase /bin/bash -c "%v"`,
		app, app, app, eB(mach), eB(tmrepo), eB(tmhead), eB(strings.Join(seeds, ",")), eB(runScript))}
	fmt.Println(args)
	if !runProcess("init-tmnode-"+mach, "docker-machine", args) {
		return errors.New("Failed to init tmnode on machine " + mach)
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
			// return "", errors.New("Failed to get tmnode validator on machine " + mach)
		} else {
			fmt.Println(Fmt("validator for %v: %v", mach, output))
			return nil
		}
	}

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
func copyToMachine(mach string, app string, srcPath string, dstPath string) error {

	// First, copy the file to a temporary location
	// in the machine.
	tempFile := "temp_" + RandStr(12)
	args := []string{"scp", "-r", srcPath, mach + ":" + tempFile}
	if !runProcess("scp-file-"+mach, "docker-machine", args) {
		return errors.New("Failed to copy file to machine " + mach)
	}

	// Next, docker cp the file into the container
	args = []string{"ssh", mach, Fmt("docker cp %v %v_tmdata:%v", tempFile, app, dstPath)}
	if !runProcess("docker-cp-file-"+mach, "docker-machine", args) {
		return errors.New("Failed to docker-cp file to container in machine " + mach)
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
