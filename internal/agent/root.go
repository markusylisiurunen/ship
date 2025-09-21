package agent

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"
)

func Execute(ctx context.Context, version string) {
	cmd := &cli.Command{
		Name:    "ship",
		Usage:   "deploy an app to a VPS",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:   "up",
				Usage:  "reconcile the machine to an up-to-date state",
				Action: NewUpAction(version).Action,
			},
			{
				Name:  "deploy",
				Usage: "deploy an app",
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "app-name", Usage: "application name", Required: true},
					&cli.StringFlag{Name: "app-version", Usage: "application version", Required: true},
				},
				Action: NewDeployAction().Action,
			},
		},
	}
	if err := cmd.Run(ctx, os.Args); err != nil {
		fmt.Printf("%v\n", err)
		os.Exit(1)
	}
}
