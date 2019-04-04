package config

import (
	"io/ioutil"

	"github.com/payfazz/go-errors"
	"gopkg.in/yaml.v2"
)

// Config .
type Config struct {
	TunName string `yaml:"tun_name"`
	Input   struct {
		Filter string `yaml:"filter"`
		Sock   string `yaml:"sock"`
	} `yaml:"input"`
	Output []struct {
		Filter string `yaml:"filter"`
		Sock   string `yaml:"sock"`
	} `yaml:"output"`
}

// Parse .
func Parse(file string) (*Config, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, errors.Wrap(err)
	}
	var c Config
	if err := yaml.UnmarshalStrict(data, &c); err != nil {
		return nil, errors.Wrap(err)
	}
	return &c, nil
}
