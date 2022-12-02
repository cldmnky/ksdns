package dynamicupdate

import (
	"fmt"

	"github.com/coredns/coredns/plugin/transfer"
)

func (d DynamicUpdate) Reload(zoneName string, t *transfer.Transfer) error {
	zone, ok := d.Zones.Z[zoneName]
	if !ok || zone == nil {
		return fmt.Errorf("zone %q not found", zoneName)
	}
	if zone.ReloadInterval == 0 {
		return nil
	}
	/* 	tick := time.NewTicker(zone.ReloadInterval)

	   	go func() {
	   		for {
	   			select {
	   			case <-tick.C:
	   				zFile := zone.File()
	   				reader, err := os.Open(filepath.Clean(zFile))
	   				if err != nil {
	   					log.Errorf("Failed to open zone %q in %q: %v", zoneName, zFile, err)
	   					continue
	   				}

	   				serial := zone.SOASerialIfDefined()
	   				z, err := file.Parse(reader, zoneName, zFile, serial)
	   				reader.Close()
	   				if err != nil {
	   					if _, ok := err.(*serialErr); !ok {
	   						log.Errorf("Parsing zone %q: %v", zoneName, err)
	   					}
	   					continue
	   				}

	   				// copy elements we need
	   				zone.Lock()
	   				zone.Apex = z.Apex
	   				zone.Tree = z.Tree
	   				zone.Unlock()

	   				log.Infof("Successfully reloaded zone %q in %q with %d SOA serial", zoneName, zFile, zone.Apex.SOA.Serial)
	   				if t != nil {
	   					if err := t.Notify(zoneName); err != nil {
	   						log.Warningf("Failed sending notifies: %s", err)
	   					}
	   				}

	   			default:
	   				tick.Stop()
	   				return
	   			}
	   		}
	   	}() */
	return nil

}
