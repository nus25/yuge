package main

import (
	_ "embed"
	"fmt"
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
								Usage:   "Bluesky identifier (handle or DID)",
								EnvVars: []string{"BLUESKY_IDENTIFIER"},
							},
							&cli.StringFlag{
								Name:    "password",
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
						},
						Action: yugeCli.PublishFeed,
					},
					{
						Name:      "unpublish",
						Aliases:   []string{"rm"},
						Usage:     "Remove a feed record from your PDS",
						ArgsUsage: "<record-key>",
						Action: func(c *cli.Context) error {
							if c.NArg() < 1 {
								return fmt.Errorf("feed URL is required")
							}
							feedURL := c.Args().Get(0)
							identifier := c.String("identifier")
							password := c.String("password")

							// TODO: フィード削除処理を実装
							fmt.Printf("Identifier: %s\n", identifier)
							fmt.Printf("Password: %s\n", password)
							fmt.Printf("Removing feed: %s\n", feedURL)
							return nil
						},
					},
					{
						Name:    "list",
						Aliases: []string{"ls"},
						Usage:   "List all feeds",
						Action: func(c *cli.Context) error {
							identifier := c.String("identifier")
							password := c.String("password")

							// TODO: フィード一覧表示処理を実装
							fmt.Printf("Identifier: %s\n", identifier)
							fmt.Printf("Password: %s\n", password)
							fmt.Println("Listing feeds...")
							return nil
						},
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
