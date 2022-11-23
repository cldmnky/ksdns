package cmd

import (
	"testing"
)

func TestRun(t *testing.T) {
	// Test runCmd
	plugins = true
	conf = "../../test/Corefile"
	t.Log("plugins: ", plugins)
	Run()

}
