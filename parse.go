package main

import (
	"errors"
	"regexp"
	"strconv"
	"strings"

	. "github.com/tendermint/go-common"
)

func ParseMachines(machsStr string) []string {
	machs, err := parseMachines(machsStr)
	if err != nil {
		Exit(err.Error())
	}
	return machs
}

// Takes a mach ranges like "foo[1,2,3];bar[3,4]baz"
// and returns ["foo1", "foo2", "foo3", "bar3baz", "bar4baz"]
func parseMachines(machsStr string) ([]string, error) {
	if len(machsStr) == 0 {
		return nil, nil
	}
	machStrs := strings.Split(machsStr, ";")
	machsMap := map[string]struct{}{}
	machs := []string{}
	for _, machStr := range machStrs {
		rangeMachs, err := parseMachinesRange(machStr)
		if err != nil {
			return nil, err
		}
		for _, mach := range rangeMachs {
			if _, ok := machsMap[mach]; ok {
				return nil, errors.New(Fmt("Duplicate machine %v", mach))
			}
			machsMap[mach] = struct{}{}
			machs = append(machs, mach)
		}
	}
	return machs, nil
}

// Takes a mach range string like "foo[1,2,3]" and returns ["foo1", "foo2", "foo3"]
func parseMachinesRange(machRange string) ([]string, error) {
	re, err := regexp.Compile(`([0-9a-zA-Z_\-.]*)\[([0-9a-zA-Z_\-.,]+)\]([0-9a-zA-Z_\-.]*)`)
	if err != nil {
		panic(err)
	}
	machParts := re.FindStringSubmatch(machRange)
	if len(machParts) == 0 {
		return []string{machRange}, nil
	}
	if len(machParts) != 4 {
		panic(Fmt("Expected 4 parts in parseMachinesRange, got %v", len(machParts)))
	}
	// fmt.Println(machParts[1], machParts[2], machParts[3])
	rangeStrs, err := expressRange(machParts[2])
	if err != nil {
		return nil, err
	}
	machines := []string{}
	for _, rangeStr := range rangeStrs {
		machines = append(machines, machParts[1]+rangeStr+machParts[3])
	}
	return machines, nil
}

// Takes a range string like "0,1,3-6" and returns ["0", "1", "3", "4", "5", "6"]
func expressRange(rangeStr string) ([]string, error) {
	rangeStrs := strings.Split(rangeStr, ",")
	expressed := []string{}
	for _, part := range rangeStrs {
		if dashIdx := strings.Index(part, "-"); dashIdx == -1 {
			expressed = append(expressed, part)
		} else {
			start := part[:dashIdx]
			end := part[dashIdx+1:]
			startNum, err := strconv.Atoi(start)
			if err != nil {
				return nil, err
			}
			endNum, err := strconv.Atoi(end)
			if err != nil {
				return nil, err
			}
			if startNum < 0 || startNum > endNum || startNum+1000 < endNum {
				return nil, errors.New(Fmt("Invalid range %v-%v", startNum, endNum))
			}
			for i := startNum; i <= endNum; i++ {
				expressed = append(expressed, Fmt("%v", i))
			}
		}
	}
	return expressed, nil
}
