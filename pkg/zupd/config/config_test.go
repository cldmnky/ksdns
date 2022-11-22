package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig(t *testing.T) {
	for scenario, fn := range map[string]func(
		*testing.T){
		"NewConfigDefaults": testNewConfigDefaults,
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
		[]string{},
		"",
		"",
		"",
	)
	assert.NoError(t, err, "Failed to create config")
	assert.Equal(t, c.Addr, "0.0.0.0", "BindAddress should be 0.0.0.0")
	assert.Equal(t, c.Port, "53", "BindPort should be 53")
}
