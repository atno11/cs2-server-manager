package main

import (
	"os"

	"cserver/internal/interfaces/cli"
)

func main() {
	os.Exit(cli.Run(
		os.Args[1:],
		os.Stdout,
		os.Stderr,
	))
}
