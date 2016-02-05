package main

import (
	"errors"
	"fmt"
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
			Name:      "init",
			Usage:     "Initialize node configuration directories",
			ArgsUsage: "[baseDir]",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "machines",
					Value: "mach[1-4]",
					Usage: "Comma separated list of machine names",
				},
			},
			Action: func(c *cli.Context) {
				cmdInit(c)
			},
		},
		{
			Name:      "create",
			Usage:     "Create a new Tendermint network with newly provisioned machines",
			ArgsUsage: "",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "machines",
					Value: "mach[1-4]",
					Usage: "Comma separated list of machine names",
				},
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
				cli.StringFlag{
					Name:  "machines",
					Value: "mach[1-4]",
					Usage: "Comma separated list of machine names",
				},
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
					Name:  "machines",
					Value: "mach[1-4]",
					Usage: "Comma separated list of machine names",
				},
				cli.StringFlag{
					Name:  "seed-machines",
					Value: "",
					Usage: "Comma separated list of machine names for seed, defaults to --machines",
				},
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
				cli.StringFlag{
					Name:  "machines",
					Value: "mach[1-4]",
					Usage: "Comma separated list of machine names",
				},
			},
			Action: func(c *cli.Context) {
				cmdStop(c)
			},
		},
		{
			Name:  "rm",
			Usage: "Remove blockchain application",
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "machines",
					Value: "mach[1-4]",
					Usage: "Comma separated list of machine names",
				},
				cli.BoolFlag{
					Name:  "force",
					Usage: "Force stop app if already running",
				},
			},
			Action: func(c *cli.Context) {
				cmdRm(c)
			},
		},
	}
	app.Run(os.Args)

}

//--------------------------------------------------------------------------------

// Initialize directories for each node
func cmdInit(c *cli.Context) {
	args := c.Args()
	if len(args) != 1 {
		cli.ShowAppHelp(c)
		return
	}
	base := args[0]
	machines := ParseMachines(c.String("machines"))

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

	genVals := make([]types.GenesisValidator, len(machines))

	// Initialize core dir and priv_validator.json's
	for i, mach := range machines {
		err := initMachCoreDirectory(base, mach)
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
		genDoc.SaveAs(path.Join(base, mach, "core", "genesis.json"))
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

func ensurePrivValidator(file string) {
	if FileExists(file) {
		return
	}
	privValidator := types.GenPrivValidator()
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
# Edit this script before "mintnet deploy" to change
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

	// Get machine ips
	seeds := make([]string, len(seedMachines))
	for i, mach := range seedMachines {
		ip, err := getMachineIP(mach)
		if err != nil {
			Exit(err.Error())
		}
		seeds[i] = ip + ":46656"
	}

	// Initialize TMCommon, TMApp, and TMNode container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			startTMCommon(mach, app)
			copyNodeDir(mach, app, base)
			startTMData(mach, app)
			startTMApp(mach, app)
			startTMNode(mach, app, seeds)
		}(mach)
	}
	wg.Wait()
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

func startTMNode(mach, app string, seeds []string) error {
	proxyApp := Fmt("tcp://%v_tmapp:46658", app)
	tmRoot := "/data/tendermint/core"
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmnode --volumes-from %v_tmcommon -d `+
		`--link %v_tmapp -p 46656:46656 -p 46657:46657 `+
		`-e TMNAME="%v" -e TMSEEDS="%v" -e TMROOT="%v" -e PROXYAPP="%v" `+
		`tendermint/tmbase /data/tendermint/core/init.sh`,
		app, app, app, eB(mach), eB(strings.Join(seeds, ",")), tmRoot, eB(proxyApp))}
	if !runProcess("start-tmnode-"+mach, "docker-machine", args) {
		return errors.New("Failed to start tmnode on machine " + mach)
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
	args := []string{"ssh", mach, Fmt(`docker rm %v_tmcommon`, app)}
	if !runProcess("rm-tmcommon-"+mach, "docker-machine", args) {
		return errors.New("Failed to rm tmcommon on machine " + mach)
	}
	return nil
}

func rmTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker rm %v_tmapp`, app)}
	if !runProcess("rm-tmapp-"+mach, "docker-machine", args) {
		return errors.New("Failed to rm tmapp on machine " + mach)
	}
	return nil
}

func rmTMNode(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker rm %v_tmnode`, app)}
	if !runProcess("rm-tmnode-"+mach, "docker-machine", args) {
		return errors.New("Failed to rm tmnode on machine " + mach)
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
