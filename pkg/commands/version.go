package commands

import (
	"fmt"

	"github.com/psviderski/uncloud-dns/pkg/version"
	"github.com/urfave/cli/v2"
)

func execute(c *cli.Context) error {
	fmt.Printf("%s\n", version.Get())

	return nil
}

func versionCommand() *cli.Command {
	return &cli.Command{
		Name:   "version",
		Usage:  "Print version",
		Action: execute,
	}
}
