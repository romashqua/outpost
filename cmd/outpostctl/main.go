package main

import (
	"fmt"
	"os"

	"github.com/romashqua-labs/outpost/pkg/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version":
		fmt.Printf("outpostctl %s\n", version.Version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`outpostctl - Outpost VPN management CLI

Usage:
  outpostctl <command> [flags]

Commands:
  version     Print version information
  help        Show this help message

More commands will be added in future releases.`)
}
