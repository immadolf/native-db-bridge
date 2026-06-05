package main

import (
	"fmt"
	"os"

	"native-db-bridge-mcp/internal/cli"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: native-db-bridge-mcp <%v>\n", cli.CommandNames())
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve", "healthcheck", "install-service", "uninstall-service":
		fmt.Printf("%s is not implemented in this task\n", os.Args[1])
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(2)
	}
}
