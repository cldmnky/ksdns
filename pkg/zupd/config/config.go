package config

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

type Config struct {
	Addr           string
	Port           string
	ZoneFiles      []ZoneFile
	ZoneDir        string
	Secret         string
	ReloadInterval time.Duration
	ChangeLogDir   string
}

type ZoneFile struct {
	Origin   string
	FileName string
}

func NewConfig(addr, port, secret string, zoneDefs []string, reload, zoneDir, changeLogDir string) (*Config, error) {
	// reload time, disable reload if 0
	if reload == "" {
		reload = "0"
	}
	if addr == "" {
		addr = "0.0.0.0"
	}
	if port == "" {
		port = "53"
	}
	d, err := time.ParseDuration(reload)
	if err != nil {
		return nil, fmt.Errorf("failed to parse reload time: %s", err.Error())
	}

	zf := make([]ZoneFile, 0)
	for _, def := range zoneDefs {
		if len(def) == 0 {
			continue
		}
		origin, fileName, err := parseZoneDef(def)
		if err != nil {
			return nil, err
		}
		zf = append(zf, ZoneFile{Origin: origin, FileName: fileName})
	}
	if zoneDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
		zoneDir = wd
	}

	return &Config{
		Addr:           addr,
		Port:           port,
		ZoneFiles:      zf,
		Secret:         secret,
		ZoneDir:        zoneDir,
		ReloadInterval: d,
		ChangeLogDir:   changeLogDir,
	}, nil
}

func (c *Config) GetOrigins() []string {
	origins := make([]string, 0)
	for _, zf := range c.ZoneFiles {
		origins = append(origins, zf.Origin)
	}
	return origins
}

func parseZoneDef(def string) (string, string, error) {
	// check that we have a ":" separator
	if !strings.Contains(def, ":") {
		return "", "", fmt.Errorf("invalid zone definition: %s", def)
	}
	// check that we have at least one character before and after the ":"
	if len(def) < 3 {
		return "", "", fmt.Errorf("invalid zone definition: %s", def)
	}
	// return the two strings
	return strings.Split(def, ":")[0], strings.Split(def, ":")[1], nil
}
