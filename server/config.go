package server

import (
	"io/ioutil"
	"log"

	toml "github.com/pelletier/go-toml"
)

type DbConfig struct {
	Source string
}

type MailConfig struct {
	Server   string
	Username string
	Password string
	ErrorTo  string `toml:"error_to"`
}

type PagesConfig struct {
	Path    string
	Buttons []string
}

type ServerConfig struct {
	Port int
}

type AppConfig struct {
	Database DbConfig
	Mail     MailConfig
	Pages    PagesConfig
	Server   ServerConfig
}

var Config AppConfig

func ReadConfig(path string) {
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		log.Fatalln("could not read config", path, err)
	}
	var config AppConfig
	if err := toml.Unmarshal(bytes, &config); err != nil {
		log.Fatalln("could not parse config", path, err)
	}
	Config = config
}
