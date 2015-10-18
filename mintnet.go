package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	. "github.com/tendermint/tendermint/common"
	pcm "github.com/tendermint/tendermint/process"

	"github.com/codegangsta/cli"
)

func main() {
	app := cli.NewApp()
	app.Name = "mintnet"
	app.Usage = "mintnet [command] [args...]"
	app.Commands = []cli.Command{
		{
			Name:  "create",
			Usage: "Create a new Tendermint network with newly provisioned machines",
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "nodes",
					Value: 4,
					Usage: "4 or more nodes",
				},
				cli.StringFlag{
					Name:  "prefix",
					Value: "testnode",
					Usage: "node name prefix",
				},
				cli.StringFlag{
					Name:  "repo",
					Value: "github.com/tendermint/tendermint",
					Usage: "repository to pull",
				},
				cli.StringFlag{
					Name:  "head",
					Value: "origin/develop",
					Usage: "branch/commit-hash to make & run",
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

// Create a new Tendermint network with newly provisioned machines
func cmdCreate(c *cli.Context) {
	args := c.Args() // Args to docker-machine
	prefix := c.String("prefix")
	numNodes := c.Int("nodes")

	// Provision numNodes machines
	errs := provisionMachines(prefix, numNodes, args)
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Successfully deployed %v machines", numNodes))
	}

	// Run containers.
	// Pull repo to given head.
	repo := c.String("repo")
	head := c.String("head")
	infos, errs := initMachines(prefix, numNodes, repo, head)
	if len(errs) > 0 {
		Exit(Fmt("There were %v errors", len(errs)))
	} else {
		fmt.Println(Fmt("Successfully initialized %v machines", numNodes))
	}

	fmt.Println(Green("Infos", infos))
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
// repo: repository to pull from, e.g. github.com/tendermint/tendermint
// head: git commit hash to make and run
func initMachines(prefix string, numNodes int, repo string, head string) (infos []string, errs []error) {
	var wg sync.WaitGroup
	for i := 1; i <= numNodes; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			info, err := initMachine(Fmt("%v-%v", prefix, i), repo, head)
			if err != nil {
				errs = append(errs, err)
			} else {
				infos = append(infos, info)
			}
		}(i)
	}
	wg.Wait()

	return infos, errs
}

// Initialize a new machine using docker-machine.
// name: name of machine
// repo: repository to pull from, e.g. github.com/tendermint/tendermint
// head: git commit hash to make and run
func initMachine(name string, repo string, head string) (info string, err error) {
	// Initialize the tmdata container
	args := []string{"ssh", name, "docker run --name tmdata --entrypoint /bin/echo tendermint/tmbase Data-only container for node"}
	if !runProcess("init-tmdata-"+name, "docker-machine", args) {
		return "", errors.New("Failed to init tmdata on machine " + name)
	}

	// Initialize the tmnode container
	args = []string{"ssh", name, Fmt("docker run --name tmnode --volumes-from tmdata -d -p 46656:46656 -p 46657:46657 -e TMREPO=\"%v\" -e TMHEAD=\"%v\" tendermint/tmbase", repo, head)}
	if !runProcess("init-tmnode-"+name, "docker-machine", args) {
		return "", errors.New("Failed to init tmnode on machine " + name)
	}

	// Get the node's validator info
	// Need to retry to wait until tendermint is installed
	for {
		args = []string{"ssh", name, Fmt("docker exec tmnode tendermint show_validator --log_level=error")}
		output, ok := runProcessGetResult("show-validator-tmnode-"+name, "docker-machine", args)
		if !ok || output == "" {
			fmt.Println(Yellow(Fmt("tendermint not yet installed in %v. Waiting...", name)))
			time.Sleep(time.Second * 5)
			// return "", errors.New("Failed to get tmnode validator on machine " + name)
		} else {
			fmt.Println(Blue(Fmt("validator for %v: %v", name, output)))
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

//--------------------------------------------------------------------------------

func runProcess(label string, command string, args []string) bool {
	outFile := NewBufferCloser(nil)
	fmt.Println(Green(command), Green(args))
	proc, err := pcm.Create(label, command, args, nil, outFile)
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
	proc, err := pcm.Create(label, command, args, nil, outFile)
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
