package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/urfave/cli/v2"
)

// ListFeeds is a CLI command handler that lists Bluesky feed generator records with optional yugeFeed field for a user.
func ListFeeds(cctx *cli.Context) error {
	debug := cctx.Bool("debug")
	detailed := cctx.Bool("detailed")
	host := cctx.String("host")
	logLevel := slog.LevelInfo
	recordKey := cctx.Args().Get(0)
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Get credentials
	identifier := cctx.String("identifier")
	if identifier == "" {
		var err error
		identifier, err = promptForInput("Bluesky identifier (handle or DID)")
		if err != nil {
			return err
		}
	}

	parsedIdentifier, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return fmt.Errorf("invalid identifier: %w", err)
	}

	password := cctx.String("password")
	if password == "" {
		var err error
		password, err = promptForPassword("Bluesky password")
		if err != nil {
			return err
		}
	}

	logger.Debug("input parameters",
		"identifier", parsedIdentifier.String(),
		"recordKey", recordKey)

	// Create session
	ctx := cctx.Context
	client, err := createSessionWithClient(ctx, host, parsedIdentifier.String(), password, logger)
	if err != nil {
		return err
	}
	defer cleanupSessionWithClient(ctx, client, logger)

	return listFeedsWithClient(ctx, client, recordKey, detailed, logger)
}

func listAllRecords(ctx context.Context, client ATProtoClient, collection string, logger *slog.Logger) ([]*repoRecordWithRawMessage, error) {
	var (
		allRecords []*repoRecordWithRawMessage
		cursor     *string = nil
	)
	logger.Debug("listing all records", "did", client.GetDID(), "collection", collection)
	for {
		logger.Debug("fetching records batch", "cursor", cursor)
		feedsOutput, err := client.ListRecords(ctx, client.GetDID(), collection, 100, cursor, false)
		if err != nil {
			logger.Error("failed to list feed generator records", "error", err)
			return nil, fmt.Errorf("failed to list feed generator records: %w", err)
		}

		allRecords = append(allRecords, feedsOutput.Records...)

		if feedsOutput.Cursor == nil {
			break
		}
		logger.Debug("fetched records batch", "count", len(feedsOutput.Records), "nextCursor", *feedsOutput.Cursor)
		cursor = feedsOutput.Cursor
	}
	logger.Debug("completed listing all records", "totalCount", len(allRecords))

	return allRecords, nil
}

func listFeedsWithClient(ctx context.Context, client ATProtoClient, recordKey string, detailed bool, logger *slog.Logger) error {
	// Get all "app.bsky.feed.generator" records of the user
	records, err := listAllRecords(ctx, client, COLLECTION_TYPE_FEED_GENERATOR, logger)
	if err != nil {
		return err
	}

	// Filter records by recordKey pattern if provided (supports wildcards: ? and *)
	var filteredRecords []*repoRecordWithRawMessage
	if recordKey != "" {
		found := false
		for _, record := range records {
			a, err := syntax.ParseATURI(record.Uri)
			if err != nil {
				logger.Error("failed to parse record URI", "uri", record.Uri, "error", err)
				fmt.Fprintf(os.Stderr, "Warning: Skipping malformed URI: %s\n", record.Uri)
				continue
			}
			// Use filepath.Match for shell-style wildcard matching (? and *)
			matched, err := filepath.Match(recordKey, string(a.RecordKey()))
			if err != nil {
				return fmt.Errorf("invalid pattern %q: %w", recordKey, err)
			}
			if matched {
				found = true
				filteredRecords = append(filteredRecords, record)
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "No matching feed records found: %s\n", recordKey)
			return nil
		} else {
			records = filteredRecords
		}
	}

	for _, record := range records {
		if detailed {
			// Show detailed JSON
			var prettyJSON bytes.Buffer
			if err := json.Indent(&prettyJSON, record.Value, "", "  "); err != nil {
				logger.Warn("failed to format JSON, showing raw", "uri", record.Uri, "error", err)
				fmt.Fprintf(os.Stderr, "Warning: Failed to format JSON for %s, showing raw\n", record.Uri)
				fmt.Printf("%s (raw): %s\n", record.Uri, string(record.Value))
			} else {
				fmt.Printf("%s:\n%s\n", record.Uri, prettyJSON.String())
			}
		} else {
			// Show only record URI
			fmt.Println(record.Uri)
		}
	}

	return nil
}
