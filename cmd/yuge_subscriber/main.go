package main

import (
	_ "embed"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3" // Register the SQLite driver for upcoming SQLite-backed persistence.
	"github.com/nus25/yuge/subscriber"
	"github.com/urfave/cli/v2"
)

//go:embed version.txt
var version string

func main() {
	run(os.Args)
}

func run(args []string) {
	app := cli.App{
		Name:    "Yuge subscriber",
		Usage:   "jetstream subscriber for bluesky custom feeds",
		Version: version,
		Commands: []*cli.Command{
			{
				Name:   "run",
				Usage:  "Run the jetstream subscriber",
				Action: subscriber.JetstreamSubscriber,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "log-level",
						Aliases: []string{"l"},
						Value:   "info",
						Usage:   "Set log level (debug, info, warn, error)",
						EnvVars: []string{"LOG_LEVEL"},
					},
					&cli.StringFlag{
						Name:     "feed-editor-endpoint",
						Usage:    "endpoint url for gyoka editor",
						EnvVars:  []string{"FEED_EDITOR_ENDPOINT"},
						Required: false,
					},
					&cli.StringFlag{
						Name:    "feed-editor-cf-id",
						Usage:   "Cloudflare access id",
						Value:   "",
						EnvVars: []string{"CF_ACCESS_CLIENT_ID"},
					},
					&cli.StringFlag{
						Name:    "feed-editor-cf-secret",
						Usage:   "Cloudflare access secret",
						Value:   "",
						EnvVars: []string{"CF_ACCESS_CLIENT_SECRET"},
					},
					&cli.StringFlag{
						Name:    "gyoka-api-key",
						Usage:   "Gyoka API key",
						Value:   "",
						EnvVars: []string{"GYOKA_API_KEY"},
					},
					&cli.StringFlag{
						Name:    "jetstream-url",
						Usage:   "full websocket path to the jetstream endpoint",
						Value:   "ws://localhost:6009/subscribe",
						EnvVars: []string{"JETSTREAM_WS_URL"},
					},
					&cli.Int64Flag{
						Name:    "override-cursor",
						Usage:   "override cursor value for jetstream",
						Value:   -1,
						EnvVars: []string{"OVERRIDE_CURSOR"},
					},
					&cli.BoolFlag{
						Name:    "jetstream-commpression",
						Usage:   "enable compression of jetstream",
						Value:   true,
						EnvVars: []string{"JETSTREAM_COMPRESSION"},
					},
					&cli.StringFlag{
						Name:    "config-directory-path",
						Usage:   "config directory path",
						Value:   "./config",
						EnvVars: []string{"CONFIG_DIR"},
					},
					&cli.StringFlag{
						Name:    "data-directory-path",
						Usage:   "data directory path",
						Value:   "./data",
						EnvVars: []string{"DATA_DIR"},
					},
					&cli.BoolFlag{
						Name:    "import-legacy-store-json",
						Usage:   "import legacy FileEditor store.json snapshots into SQLite before startup",
						Value:   false,
						EnvVars: []string{"IMPORT_LEGACY_STORE_JSON"},
					},
					&cli.BoolFlag{
						Name:    "import-legacy-store-enqueue-projection",
						Usage:   "enqueue imported legacy posts into projection_outbox for replay during startup",
						Value:   false,
						EnvVars: []string{"IMPORT_LEGACY_STORE_ENQUEUE_PROJECTION"},
					},
					&cli.StringFlag{
						Name:    "api-listen-addr",
						Usage:   "addr to serve prometheus metrics on",
						Value:   ":8082",
						EnvVars: []string{"SUBSCRIBER_API_LISTEN_ADDR"},
					},
					&cli.StringFlag{
						Name:    "metrics-listen-addr",
						Usage:   "addr to serve prometheus metrics on",
						Value:   ":9102",
						EnvVars: []string{"SUBSCRIBER_METRICS_LISTEN_ADDR"},
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
