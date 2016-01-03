package main

import (
	"strings"
	"testing"
)

func TestParseEmpty(t *testing.T) {
	machs, err := parseMachines("")
	if err != nil {
		t.Fatal("Unexpected error:", err)
	}
	if len(machs) != 0 {
		t.Error("Expected to get zero machines, got ", len(machs))
	}
}

func TestParseRange(t *testing.T) {
	expect(t, "foo", "foo")
	expect(t, "foo;bar", "foo%bar")
	expect(t, "foo[1]", "foo1")
	expect(t, "foo[1,2]", "foo1%foo2")
	expect(t, "foo[1,2]bar", "foo1bar%foo2bar")
	expect(t, "foo[1,2]bar;baz", "foo1bar%foo2bar%baz")
	expect(t, "foo[1,2,5-7]bar;baz", "foo1bar%foo2bar%foo5bar%foo6bar%foo7bar%baz")
	expect(t, "foo[0-2];foo[4-6]", "foo0%foo1%foo2%foo4%foo5%foo6")
	expect(t, "[0-2];[4-6]", "0%1%2%4%5%6")
}

func expect(t *testing.T, machsStr string, machsExpected string) {
	machsGot, err := parseMachines(machsStr)
	if err != nil {
		t.Errorf("Failed to parse: %v", err)
		return
	}
	if strings.Join(machsGot, "%") != machsExpected {
		t.Errorf("Expected %v but got %v", machsExpected, machsGot)
	}
}
