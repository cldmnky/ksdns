package config

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	for scenario, fn := range map[string]func(
		*testing.T){
		"NewConfigDefaults": testNewConfigDefaults,
		"GetOrigins":        testGetOrigins,
	} {
		t.Run(scenario, func(t *testing.T) {
			fn(t)
		})
	}
}

func testNewConfigDefaults(t *testing.T) {
	c, err := NewConfig(
		"",
		"",
		"",
		[]string{"example.com.:example.com"},
		"",
		"",
		"",
	)
	assert.NoError(t, err, "Failed to create config")
	assert.Equal(t, c.Addr, "0.0.0.0", "BindAddress should be 0.0.0.0")
	assert.Equal(t, c.Port, "53", "BindPort should be 53")
	assert.Equal(t, c.ReloadInterval, time.Duration(0), "ReloadInterval should be 0")
	wd, err := os.Getwd()
	assert.NoError(t, err, "Failed to get working directory")
	assert.Equal(t, c.ZoneDir, wd, fmt.Sprintf("ZoneDir should be %s", wd))
	assert.Equal(t, c.ZoneFiles[0].Origin, "example.com.", "Origin should be example.com.")
	assert.Equal(t, c.ZoneFiles[0].FileName, "example.com", "FileName should be example.com")
	assert.Equal(t, c.Secret, "", "Secret should be empty")
	assert.Equal(t, c.ChangeLogDir, "", "ChangeLogDir should be empty")
}

func testGetOrigins(t *testing.T) {
	c, err := NewConfig(
		"",
		"",
		"",
		[]string{"example.com.:example.com"},
		"",
		"",
		"",
	)
	assert.NoError(t, err, "Failed to create config")
	origins := c.GetOrigins()
	assert.Equal(t, origins[0], "example.com.", "Origin should be example.com.")
}
