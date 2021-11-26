package main

import (
	"embed"
	"io/fs"
	"log"

	"github.com/heijmans/independ/server"
)

//go:embed public/*
var embeddedFs embed.FS

const CONFIG_PATH = "config.toml"

func main() {
	server.ReadConfig(CONFIG_PATH)
	server.SetupDb()

	publicFs, err := fs.Sub(embeddedFs, "public")
	if err != nil {
		log.Panicln("could get public folder", err)
	}
	server.Serve(publicFs)
}
