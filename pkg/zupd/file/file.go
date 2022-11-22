package file

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cldmnky/ksdns/pkg/zupd/changelog"
	"github.com/cldmnky/ksdns/pkg/zupd/config"
	corefile "github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/plugin/transfer"
	"github.com/miekg/dns"

	log "github.com/sirupsen/logrus"
)

type (
	// File is the plugin that reads zone data from disk.
	File struct {
		Zones
		transfer *transfer.Transfer
	}

	// Zones maps zone names to a *Zone.
	Zones struct {
		Z         map[string]*corefile.Zone // A map mapping zone (origin) to the Zone's data
		Names     []string                  // All the keys from the map Z as a string slice.
		ChangeLog map[string]*changelog.Log
	}
)

func NewFile(config *config.Config) (*File, error) {
	var (
		zf   string
		zcld string
	)
	f := &File{
		Zones: Zones{
			Z:         make(map[string]*corefile.Zone),
			Names:     make([]string, 0),
			ChangeLog: make(map[string]*changelog.Log),
		},
	}

	origins := config.GetOrigins()
	f.Zones.Names = origins

	for _, zoneFile := range config.ZoneFiles {
		if !filepath.IsAbs(zoneFile.FileName) {
			zf = filepath.Join(config.ZoneDir, zoneFile.FileName)
		} else {
			zf = zoneFile.FileName
		}
		reader, err := os.Open(filepath.Clean(zf))
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		z, err := Parse(reader, zoneFile.Origin, zf, 0)
		if err != nil {
			return nil, err
		}
		f.Zones.Z[zoneFile.Origin] = z
	}
	for i, name := range f.Zones.Names {
		f.Zones.Z[origins[i]].ReloadInterval = config.ReloadInterval
		if config.ChangeLogDir != "" {
			zcld = filepath.Join(config.ChangeLogDir, name)
			// Make directory if it doesn't exist
			if _, err := os.Stat(zcld); os.IsNotExist(err) {
				err := os.MkdirAll(zcld, 0755)
				if err != nil {
					return nil, err
				}
			}

			c := changelog.Config{}
			c.Segment.MaxStoreBytes = 1024 * 1024
			l, err := changelog.NewLog(zcld, c)
			if err != nil {
				return nil, err
			}
			f.Zones.ChangeLog[name] = l
		}
	}
	// TODO: Replay log on startup
	return f, nil
}

func (f *File) GetZone(name string) *corefile.Zone {
	if z, ok := f.Zones.Z[name]; ok {
		return z
	}
	return nil
}

func (f *File) GetChangeLog(name string) *changelog.Log {
	if l, ok := f.Zones.ChangeLog[name]; ok {
		return l
	}
	return nil
}

// Insert inserts a record into the zone. It returns an error if the record is not valid, and adds the record to the change log.
func (f *File) Insert(zoneName string, rr dns.RR) error {
	if z, ok := f.Zones.Z[zoneName]; ok {
		if err := z.Insert(rr); err != nil {
			return err
		}
		if l, ok := f.Zones.ChangeLog[zoneName]; ok {
			_, err := l.Append(changelog.NewRecord(zoneName, rr, changelog.TypeInsert))
			if err != nil {
				log.Debugf("Error appending record to changelog: %s", err)
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("changelog for zone %s not found", zoneName)
}

// Delete removes a record from the zone. It returns an error if the record is not valid, and adds the record to the change log.
func (f *File) Delete(zoneName string, rr dns.RR) error {
	if z, ok := f.Zones.Z[zoneName]; ok {
		z.Delete(rr)
		if l, ok := f.Zones.ChangeLog[zoneName]; ok {
			_, err := l.Append(changelog.NewRecord(zoneName, rr, changelog.TypeRemove))
			if err != nil {
				log.Debugf("Error appending record to changelog: %s", err)
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("zone %s not found", zoneName)
}

func (f *File) Replay(zoneName string) error {
	if l, ok := f.Zones.ChangeLog[zoneName]; ok {
		log.Debugf("Replaying changelog for zone %s", zoneName)
		reader := l.Reader()
		if _, err := io.Copy(os.Stdout, reader); err != nil {
			log.Fatal(err)
		}
		return nil
	}
	return fmt.Errorf("zone %s not found", zoneName)
}

func Parse(f io.Reader, origin, fileName string, serial int64) (*corefile.Zone, error) {
	zp := dns.NewZoneParser(f, dns.Fqdn(origin), fileName)
	zp.SetIncludeAllowed(true)
	z := corefile.NewZone(origin, fileName)
	seenSOA := false
	for rr, ok := zp.Next(); ok; rr, ok = zp.Next() {
		if err := zp.Err(); err != nil {
			return nil, err
		}
		if !seenSOA {
			if s, ok := rr.(*dns.SOA); ok {
				seenSOA = true
				if serial >= 0 && s.Serial == uint32(serial) {
					return nil, fmt.Errorf("zone SOA is not changed, %s already has serial %d", origin, serial)
				}
			}
		}
		if err := z.Insert(rr); err != nil {
			return nil, err
		}
	}
	if !seenSOA {
		return nil, fmt.Errorf("zone %s has no SOA record", origin)
	}
	return z, nil
}
