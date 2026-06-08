package main

import (
	"os"

	"native-db-bridge-mcp/internal/cli"
)

func main() {
	os.Exit(cli.Dispatch(os.Args))
}
