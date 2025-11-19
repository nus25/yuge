package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/urfave/cli/v2"
)

// UnpublishFeed is a CLI command handler that removes a Bluesky feed generator record.
func UnpublishFeed(cctx *cli.Context) error {
	// Setup logger
	debug := cctx.Bool("debug")
	force := cctx.Bool("force")
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Parse record key
	if cctx.NArg() < 1 {
		return fmt.Errorf("feed record name is required")
	}
	recordKey := cctx.Args().Get(0)

	// Validate record key
	if _, err := syntax.ParseRecordKey(recordKey); err != nil {
		logger.Error("invalid record key", "error", err)
		return fmt.Errorf("invalid record key: %w", err)
	}
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

	host := cctx.String("host")
	if host == "" {
		host = "https://bsky.social"
	}

	logger.Debug("input parameters",
		"identifier", parsedIdentifier,
		"recordKey", recordKey,
		"host", host,
	)

	// Create session
	ctx := cctx.Context
	client, err := createSessionWithClient(ctx, host, parsedIdentifier.String(), password, logger)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer cleanupSessionWithClient(ctx, client, logger)

	// Delete the feed record
	return unpublishFeedWithClient(ctx, client, recordKey, logger, force)
}

// unpublishFeedWithClient removes a feed record using the ATProtoClient interface
func unpublishFeedWithClient(ctx context.Context, client ATProtoClient, recordKey string, logger *slog.Logger, force bool) error {
	logger.Info("checking if feed record exists", "recordKey", recordKey)
	// Check if record exists
	_, err := client.GetRecord(ctx, COLLECTION_TYPE_FEED_GENERATOR, client.GetDID(), recordKey)
	if err != nil {
		if strings.Contains(err.Error(), "RecordNotFound") {
			logger.Error("feed record not found", "recordKey", recordKey, "error", err)
			return fmt.Errorf("feed record '%s' not found", recordKey)
		}
		logger.Error("failed to fetch feed record", "recordKey", recordKey, "error", err)
		return fmt.Errorf("failed to fetch feed record: %w", err)
	}
	logger.Info("feed record found, proceeding to delete", "recordKey", recordKey)

	if force {
		logger.Info("force flag detected; skipping confirmation prompt", "recordKey", recordKey)
	} else {
		confirmed, err := promptConfirmation(fmt.Sprintf("Are you sure you want to delete feed record '%s'?", recordKey))
		if err != nil {
			logger.Warn("failed to read user input", "error", err)
			return err
		}
		if !confirmed {
			logger.Info("unpublish cancelled by user", "recordKey", recordKey)
			fmt.Println("Cancelled unpublish operation.")
			return nil
		}
	}

	// Delete the record
	input := &atproto.RepoDeleteRecord_Input{
		Repo:       client.GetDID(),
		Collection: COLLECTION_TYPE_FEED_GENERATOR,
		Rkey:       recordKey,
	}

	logger.Info("deleting feed record from PDS", "recordKey", recordKey)

	if _, err := client.DeleteRecord(ctx, input); err != nil {
		logger.Error("failed to delete record", "error", err)
		return fmt.Errorf("failed to delete record: %w", err)
	}

	logger.Info("feed record deleted successfully", "recordKey", recordKey)
	fmt.Printf("Successfully unpublished feed record: %s\n", recordKey)

	return nil
}
