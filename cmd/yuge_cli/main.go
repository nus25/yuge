package main

import (
	_ "embed"
	"log"
	"os"

	yugeCli "github.com/nus25/yuge/cli"
	"github.com/urfave/cli/v2"
)

//go:embed version.txt
var version string

func main() {
	run(os.Args)
}

func run(args []string) {
	app := &cli.App{
		Name:    "Yuge CLI",
		Usage:   "Command line interface for Yuge",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:  "feed",
				Usage: "Manage feeds",
				Subcommands: []*cli.Command{
					{
						Name:      "publish",
						Usage:     "Write a new feed record to your PDS",
						ArgsUsage: "<record-key>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "identifier",
								Aliases: []string{"i"},
								Usage:   "Bluesky identifier (handle or DID)",
								EnvVars: []string{"BLUESKY_IDENTIFIER"},
							},
							&cli.StringFlag{
								Name:    "password",
								Aliases: []string{"p"},
								Usage:   "Bluesky password",
								EnvVars: []string{"BLUESKY_PASSWORD"},
							},
							&cli.StringFlag{
								Name:     "service-did",
								Usage:    "Feed Generator service DID",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "display-name",
								Usage:    "Display name for the feed. maxLength: 240",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "description",
								Usage: "Description of the feed. maxLength: 3000",
							},
							&cli.PathFlag{
								Name:  "avatar",
								Usage: "Path to avatar image file. maxSize: 1000000 bytes format: [png, jpeg]",
							},
							&cli.StringFlag{
								Name:  "content-mode",
								Value: "unspecified",
								Usage: "type of view mode. [unspecified, video]",
							},
							&cli.BoolFlag{
								Name:  "accepts-interactions",
								Value: false,
								Usage: "Whether to accept interaction feedback from viewers.",
							},
							&cli.PathFlag{
								Name:  "yuge-config",
								Usage: "path to yuge feed config YAML file",
							},
							&cli.StringFlag{
								Name:   "host",
								Value:  "https://bsky.social",
								Usage:  "Bluesky service host URL",
								Hidden: true,
							},
							&cli.BoolFlag{
								Name:  "debug",
								Usage: "Enable detailed debug logging",
							},
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Skip confirmation prompt",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Value: false,
								Usage: "Perform all validations and show the " +
									"resulting feed record without publishing it",
							},
						},
						Action: yugeCli.PublishFeed,
					},
					{
						Name:      "unpublish",
						Usage:     "Remove a feed record from your PDS",
						ArgsUsage: "<record-key>",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "identifier",
								Aliases: []string{"i"},
								Usage:   "Bluesky identifier (handle or DID)",
								EnvVars: []string{"BLUESKY_IDENTIFIER"},
							},
							&cli.StringFlag{
								Name:    "password",
								Aliases: []string{"p"},
								Usage:   "Bluesky password",
								EnvVars: []string{"BLUESKY_PASSWORD"},
							},
							&cli.StringFlag{
								Name:   "host",
								Value:  "https://bsky.social",
								Usage:  "Bluesky service host URL",
								Hidden: true,
							},
							&cli.BoolFlag{
								Name:  "debug",
								Usage: "Enable detailed debug logging",
							},
							&cli.BoolFlag{
								Name:    "force",
								Aliases: []string{"f"},
								Usage:   "Skip confirmation prompt",
							},
						},
						Action: yugeCli.UnpublishFeed,
					},
					{
						Name:      "list",
						Aliases:   []string{"ls"},
						Usage:     "List all feeds or specific feed records by record key",
						ArgsUsage: "[record-key]",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:    "identifier",
								Aliases: []string{"i"},
								Usage:   "Bluesky identifier (handle or DID)",
								EnvVars: []string{"BLUESKY_IDENTIFIER"},
							},
							&cli.StringFlag{
								Name:    "password",
								Aliases: []string{"p"},
								Usage:   "Bluesky password",
								EnvVars: []string{"BLUESKY_PASSWORD"},
							},
							&cli.StringFlag{
								Name:   "host",
								Value:  "https://bsky.social",
								Usage:  "Bluesky service host URL",
								Hidden: true,
							},
							&cli.BoolFlag{
								Name:  "debug",
								Usage: "Enable detailed debug logging",
							},
							&cli.BoolFlag{
								Name:    "detailed",
								Aliases: []string{"d"},
								Usage:   "Show record details",
							},
						},
						Action: yugeCli.ListFeeds,
					},
				},
			},
		},
	}

	err := app.Run(args)
	if err != nil {
		log.Fatal(err)
	}
}
