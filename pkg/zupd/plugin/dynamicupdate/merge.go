package dynamicupdate

import (
	"github.com/coredns/coredns/plugin/file"
)

// Merge the dynamic zone with the static zone. Return a new zone.
func (d DynamicUpdate) Merge(origin string) *file.Zone {
	z, ok := d.Zones.Z[origin]
	if !ok || z == nil {
		return nil
	}

	dz, ok := d.Zones.DynamicZones[origin]
	if !ok || dz == nil {
		return nil
	}
	// Lock the zones
	z.RLock()
	dz.RLock()

	// Make a copy of the base zone
	newZone := z.Copy()

	// Merge the dynamic zone with the static zone.
	for _, te := range dz.All() {
		for _, rr := range te.All() {
			newZone.Insert(rr)
		}
	}
	z.RUnlock()
	dz.RUnlock()
	return newZone
}
