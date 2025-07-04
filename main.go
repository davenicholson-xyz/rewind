/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"github.com/davenicholson-xyz/rewind/app"
	"github.com/davenicholson-xyz/rewind/cmd"
)

var version string

func main() {

	logConfig := app.LoggerConfigFromEnv()
	if err := app.InitLogger(logConfig); err != nil {
		panic("Failed to initialize logging")
	}

	app.Logger.Info("Rewind initializing")

	cmd.SetVersion(version)
	cmd.Execute()
}
