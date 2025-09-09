package main

import (
	"github.com/neptaco/unity-cli/cmd"
)

var version = "dev"

func main() {
	cmd.Execute(version)
}