package main

import (
	"fmt"

	. "github.com/tendermint/go-common"

	"github.com/codegangsta/cli"
)

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
		portMap, err := getContainerPortMap(mach, fmt.Sprintf("%v_tmcore", appName))
		if err != nil {
			Exit(err.Error())
		}
		fmt.Println("Machine", mach)
		fmt.Println(portMap)
		fmt.Println("")
	}
}
