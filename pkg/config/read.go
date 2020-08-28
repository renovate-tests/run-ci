package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

type ExistFile func(string) bool

type Reader struct {
	ExistFile ExistFile
}

func (reader Reader) find(wd string) (string, bool) {
	names := []string{".run-ci.yml", ".run-ci.yaml"}
	for {
		for _, name := range names {
			p := filepath.Join(wd, name)
			if reader.ExistFile(p) {
				return p, true
			}
		}
		if wd == "/" || wd == "" {
			return "", false
		}
		wd = filepath.Dir(wd)
	}
}

func (reader Reader) read(p string) (Config, error) {
	cfg := Config{}
	f, err := os.Open(p)
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (reader Reader) FindAndRead(cfgPath, wd string) (Config, error) {
	cfg := Config{}
	if cfgPath == "" {
		p, b := reader.find(wd)
		if !b {
			return cfg, nil
		}
		cfgPath = p
	}
	return reader.read(cfgPath)
}
