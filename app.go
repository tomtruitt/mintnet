package main

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	. "github.com/tendermint/go-common"
	client "github.com/tendermint/go-rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"

	"github.com/codegangsta/cli"
)

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
	randomPorts := c.Bool("publish-all")
	seedsStr := c.String("seeds")
	seeds := []string{}
	if seedsStr != "" {
		seeds = strings.Split(seedsStr, ",")
	}
	noTMSP := c.Bool("no-tmsp")
	tmcoreImage := c.String("tmcore-image")
	tmappImage := c.String("tmapp-image")

	// Initialize TMData, TMApp, and TMCore container on each machine
	// We let nodes boot and then detect which port they're listening on to collect CoreInfos
	var wg sync.WaitGroup
	coreInfosCh := make(chan *CoreInfo, len(machines))
	errCh := make(chan error, len(machines))
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			if err := startTMCommon(mach, app); err != nil {
				errCh <- err
				return
			}
			if err := copyNodeDir(mach, app, base); err != nil {
				errCh <- err
				return
			}

			// if noTMSP, we ignore socket and app containers
			// and just use an in-process null app
			if !noTMSP {
				if err := startTMData(mach, app); err != nil {
					errCh <- err
					return
				}
				if err := startTMApp(mach, app, tmappImage); err != nil {
					errCh <- err
					return
				}
			}

			coreInfo, err := startTMCore(mach, app, nil, randomPorts, noTMSP, tmcoreImage)
			if err != nil {
				errCh <- err
				return
			}
			coreInfosCh <- coreInfo
		}(mach)
	}
	wg.Wait()

	// Collect coreInfos, and maybe append seeds
	var coreInfos []*CoreInfo
	for i := 0; i < len(machines); i++ {
		select {
		case err := <-errCh:
			fmt.Println(Red(err.Error()))
		case coreInfo := <-coreInfosCh:
			coreInfos = append(coreInfos, coreInfo)
			if seedsStr == "" {
				seeds = append(seeds, coreInfo.P2PAddr)
			}
		}
	}

	// Dial the seeds
	fmt.Println(Green("Instruct nodes to dial each other"))
	for _, core := range coreInfos {
		wg.Add(1)
		go func(rpcAddr string) {
			defer wg.Done()
			if err := dialSeeds(rpcAddr, seeds); err != nil {
				fmt.Println(Red(err.Error()))
				return
			}
		}(core.RPCAddr)
	}
	wg.Wait()

	fmt.Println(Green("Done launching tendermint network for " + app))
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
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmcommon --entrypoint true tendermint/tmbase`, app)}
	if !runProcess("start-tmcommon-"+mach, "docker-machine", args, true) {
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
	if !runProcess("start-tmdata-"+mach, "docker-machine", args, true) {
		return errors.New("Failed to start tmdata on machine " + mach)
	}
	for i := 1; i < 20; i++ { // TODO configure
		time.Sleep(time.Duration(i) * time.Second)
		if checkFileExists(mach, app+"_tmdata", "/data/tendermint/data/data.sock") {
			return nil
		}
	}
	return errors.New("Failed to start tmdata on machine " + mach + " (timeout)")
}

func startTMApp(mach, app, image string) error {
	args := []string{"ssh", mach, Fmt(`docker run --name %v_tmapp --volumes-from %v_tmcommon -d `+
		`%v /data/tendermint/app/init.sh`, app, app, image)}
	if !runProcess("start-tmapp-"+mach, "docker-machine", args, true) {
		return errors.New("Failed to start tmapp on machine " + mach)
	}
	return nil
}

func startTMCore(mach, app string, seeds []string, randomPort, noTMSP bool, image string) (*CoreInfo, error) {
	portString := "-p 46656:46656 -p 46657:46657"
	if randomPort {
		portString = "--publish-all"
	}

	// IF APP-USE-TCP
	//   proxyApp := Fmt("tcp://%v_tmapp:46658", app)
	//   tmspConditions := Fmt(` --link %v_tmapp `, app)
	// ELSE
	proxyApp := "unix:///data/tendermint/app/app.sock"
	tmspConditions := ""
	// END

	if noTMSP {
		proxyApp = "nilapp" // in-proc nil app
		tmspConditions = "" // tmcommon and tmapp weren't started
	}
	tmRoot := "/data/tendermint/core"
	args := []string{"ssh", mach, Fmt(`docker run -d %v --name %v_tmcore --volumes-from %v_tmcommon %v`+
		`-e TMNAME="%v" -e TMSEEDS="%v" -e TMROOT="%v" -e PROXYAPP="%v" `+
		`%v /data/tendermint/core/init.sh`,
		portString, app, app, tmspConditions,
		eB(mach), eB(strings.Join(seeds, ",")), tmRoot, eB(proxyApp), image)}
	if !runProcess("start-tmcore-"+mach, "docker-machine", args, true) {
		return nil, errors.New("Failed to start tmcore on machine " + mach)
	}

	// Give it some time to install and make repo.
	time.Sleep(time.Second * 10)

	// Get the node's validator info
	// Need to retry to wait until tendermint is installed
	for {
		args = []string{"ssh", mach, Fmt(`docker exec %v_tmcore tendermint show_validator --log_level=error`, app)}
		output, ok := runProcessGetResult("show-validator-tmcore-"+mach, "docker-machine", args, false)
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

			coreInfo := &CoreInfo{
				Validator: &Validator{
					ID: mach,
				},
			}

			var p2pPort, rpcPort = "46656", "46657"
			if randomPort {
				portMap, err := getContainerPortMap(mach, fmt.Sprintf("%v_tmcore", app))
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
			coreInfo.P2PAddr = fmt.Sprintf("%v:%v", ip, p2pPort)
			coreInfo.RPCAddr = fmt.Sprintf("%v:%v", ip, rpcPort)

			// get pubkey from rpc endpoint
			// try a few times in case the rpc server is slow to start
			var result ctypes.TMResult
			for i := 0; i < 10; i++ {
				time.Sleep(time.Second)
				c := client.NewClientURI(fmt.Sprintf("%s", coreInfo.RPCAddr))
				if _, err = c.Call("status", nil, &result); err != nil {
					continue
				}
				status := result.(*ctypes.ResultStatus)
				coreInfo.Validator.PubKey = status.PubKey
				break
			}
			if err != nil {
				return nil, fmt.Errorf("Error getting PubKey from mach %s on %s: %v", mach, coreInfo.RPCAddr, err)
			}

			return coreInfo, nil
		}
	}
	return nil, nil
}

func dialSeeds(rpcAddr string, seeds []string) error {
	var result ctypes.TMResult
	c := client.NewClientURI(fmt.Sprintf("%s", rpcAddr))
	args := map[string]interface{}{"seeds": seeds}
	_, err := c.Call("dial_seeds", args, &result)
	if err != nil {
		return errors.New("Error dialing seeds at rpc address " + rpcAddr)
	}
	return nil
}

func getContainerPortMap(mach, container string) (map[string]string, error) {
	args := []string{"ssh", mach, Fmt(`docker port %v`, container)}
	output, ok := runProcessGetResult(fmt.Sprintf("get-ports-%v-%v", mach, container), "docker-machine", args, true)
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

func cmdRestart(c *cli.Context) {
	args := c.Args()
	if len(args) == 0 {
		Exit("restart requires argument for app name")
	}
	app := args[0]
	machines := ParseMachines(c.String("machines"))

	// Restart TMApp, and TMCore container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			restartTMApp(mach, app)
			restartTMCore(mach, app)
		}(mach)
	}
	wg.Wait()
}

func restartTMCore(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker start %v_tmcore`, app)}
	if !runProcess("restart-tmcore-"+mach, "docker-machine", args, true) {
		return errors.New("Failed to restart tmcore on machine " + mach)
	}
	return nil
}

func restartTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker start %v_tmapp`, app)}
	if !runProcess("restart-tmapp-"+mach, "docker-machine", args, true) {
		return errors.New("Failed to restart tmapp on machine " + mach)
	}
	return nil
}

//--------------------------------------------------------------------------------

func cmdStop(c *cli.Context) {
	args := c.Args()
	if len(args) == 0 {
		Exit("stop requires argument for app name")
	}
	app := args[0]
	machines := ParseMachines(c.String("machines"))

	// Initialize TMCommon, TMData, TMApp, and TMCore container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			stopTMCore(mach, app)
			stopTMApp(mach, app)
			stopTMData(mach, app)
		}(mach)
	}
	wg.Wait()
}

func stopTMData(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker stop %v_tmdata`, app)}
	if !runProcess("stop-tmdata-"+mach, "docker-machine", args, true) {
		return errors.New("Failed to stop tmdata on machine " + mach)
	}
	return nil
}

func stopTMCore(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker stop %v_tmcore`, app)}
	if !runProcess("stop-tmcore-"+mach, "docker-machine", args, true) {
		return errors.New("Failed to stop tmcore on machine " + mach)
	}
	return nil
}

func stopTMApp(mach, app string) error {
	args := []string{"ssh", mach, Fmt(`docker stop %v_tmapp`, app)}
	if !runProcess("stop-tmapp-"+mach, "docker-machine", args, true) {
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

	// Remove TMCommon, TMApp, and TMNode container on each machine
	var wg sync.WaitGroup
	for _, mach := range machines {
		wg.Add(1)
		go func(mach string) {
			defer wg.Done()
			rmContainer(mach, Fmt("%v_tmcommon", app), force)
			rmContainer(mach, Fmt("%v_tmdata", app), force)
			rmContainer(mach, Fmt("%v_tmapp", app), force)
			rmContainer(mach, Fmt("%v_tmcore", app), force)
		}(mach)
	}
	wg.Wait()
}

func rmContainer(mach, container string, force bool) error {
	opts := ""
	if force {
		opts = "-f"
	}
	args := []string{"ssh", mach, Fmt(`docker rm %v %v`, opts, container)}
	if !runProcess(Fmt("rm-%v-%v", container, mach), "docker-machine", args, true) {
		return errors.New(Fmt("Failed to rm %v on machine %v", container, mach))
	}
	return nil
}
