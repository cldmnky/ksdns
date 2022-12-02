package dynamicupdate

import (
	"strings"
	"testing"

	"github.com/coredns/coredns/plugin/file"
	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test Merge
func TestMerge(t *testing.T) {
	zone, err := file.Parse(strings.NewReader(exampleOrg), exampleOrgZone, "stdin", 0)
	if err != nil {
		return
	}
	dynamicZone := file.NewZone(exampleOrgZone, "")

	d := DynamicUpdate{
		Zones: &Zones{
			Z: map[string]*file.Zone{
				exampleOrgZone: zone,
			},
			DynamicZones: map[string]*file.Zone{
				exampleOrgZone: dynamicZone,
			},
		},
	}
	newZone := d.Merge(exampleOrgZone)
	require.NotNil(t, newZone)
	assert.Equal(t, len(newZone.All()), len(zone.All()))
	assert.Equal(t, len(d.Zones.DynamicZones[exampleOrgZone].All()), 0)

	// Add a record to the dynamic zone
	exampleOrgRR, err := dns.NewRR("new.example.com. 3600 IN A 127.0.0.1")
	require.NoError(t, err)
	dynamicZone.Insert(exampleOrgRR)
	assert.Equal(t, len(d.Zones.DynamicZones[exampleOrgZone].All()), 1)
	newZone = d.Merge(exampleOrgZone)
	require.NotNil(t, newZone)
	assert.Equal(t, len(newZone.All()), len(zone.All())+1)
}
