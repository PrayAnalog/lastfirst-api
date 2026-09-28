package main

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestServerEmbedsTimeZoneData(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(strings.Fields(string(out)), "time/tzdata") {
		t.Fatal("server binary does not import time/tzdata, so America/Los_Angeles cannot load in the runtime image, which has no zoneinfo")
	}
}
