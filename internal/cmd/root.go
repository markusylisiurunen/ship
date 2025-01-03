package cmd

import (
	"context"
	"os"

	"github.com/markusylisiurunen/ship/internal/log"
	"github.com/urfave/cli/v3"
)

func Execute(ctx context.Context) {
	cmd := &cli.Command{
		Commands: []*cli.Command{
			{
				Name:   "deploy",
				Usage:  "deploy application to a remote server",
				Action: newDeployAction().action,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Usage:    "application name (e.g., 'myapp')",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "version",
						Usage:    "application version or git hash (e.g., 'v1.0.0' or commit SHA)",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "host",
						Usage:    "remote server address (e.g., 'example.com' or '192.168.1.100')",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "password",
						Usage:    "ssh password for root user",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:  "volume",
						Usage: "ensures a folder exists on the remote server's '.data' folder (e.g., 'uploads')",
					},
				},
			},
			{
				Name:  "secrets",
				Usage: "manage application secrets",
				Commands: []*cli.Command{
					{
						Name:      "set",
						Usage:     "set a secret value",
						ArgsUsage: "<key> <value>",
						Action:    newSecretsSetAction().action,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "name",
								Usage:    "application name (e.g., 'myapp')",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "host",
								Usage:    "remote server address (e.g., 'example.com' or '192.168.1.100')",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Usage:    "ssh password for root user",
								Required: true,
							},
						},
					},
					{
						Name:      "del",
						Usage:     "delete a secret value",
						ArgsUsage: "<key>",
						Action:    newSecretsDelAction().action,
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "name",
								Usage:    "application name (e.g., 'myapp')",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "host",
								Usage:    "remote server address (e.g., 'example.com' or '192.168.1.100')",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "password",
								Usage:    "ssh password for root user",
								Required: true,
							},
						},
					},
				},
			},
		},
	}
	if err := cmd.Run(ctx, os.Args); err != nil {
		log.Errorf("%v", err)
		os.Exit(1)
	}
}
