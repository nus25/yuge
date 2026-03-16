package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/nus25/yuge/feed/config/provider"
	"github.com/nus25/yuge/feed/config/types"
	"github.com/urfave/cli/v2"
)

const MAX_AVATAR_SIZE_BYTES = 1 * 1024 * 1024 // 1MB
const MAX_DISPLAY_NAME_LENGTH = 240
const MAX_DESCRIPTION_LENGTH = 3000
const COLLECTION_TYPE_FEED_GENERATOR = "app.bsky.feed.generator"
const CONTENT_MODE_UNSPECIFIED = "app.bsky.feed.defs#contentModeUnspecified"
const CONTENT_MODE_VIDEO = "app.bsky.feed.defs#contentModeVideo"

// FeedParams holds the parameters for publishing a feed
type FeedParams struct {
	RecordKey           string
	Identifier          syntax.AtIdentifier
	Password            string
	ServiceDID          string
	DisplayName         string
	Description         string
	AvatarPath          string
	ContentMode         string
	AcceptsInteractions bool
	YugeConfigPath      string
	DryRun              bool
	Host                string
}

// AvatarData holds avatar file information
type AvatarData struct {
	Data     []byte
	MimeType string
}

type comAtprotoFeedGeneratorWithYugeFeed struct {
	bsky.FeedGenerator
	YugeFeed types.FeedConfig `json:"yugeFeed,omitempty"`
}

// PublishFeed is a CLI command handler that publishes a Bluesky feed generator record with optional yugeFeed field.
func PublishFeed(cctx *cli.Context) error {
	// Setup logger based on debug flag
	debug := cctx.Bool("debug")
	force := cctx.Bool("force")
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Parse and validate inputs
	params, err := parseFeedParams(cctx)
	if err != nil {
		return err
	}

	logger.Debug("input parameters",
		"identifier", params.Identifier,
		"recordKey", params.RecordKey,
		"serviceDID", params.ServiceDID,
		"displayName", params.DisplayName,
		"description", params.Description,
		"avatarPath", params.AvatarPath,
		"contentMode", params.ContentMode,
		"acceptsInteractions", params.AcceptsInteractions,
		"yugeConfigPath", params.YugeConfigPath,
		"host", params.Host,
		"dryRun", params.DryRun,
	)

	// Validate inputs
	if err := validateFeedParams(params, logger); err != nil {
		return err
	}

	// Load avatar if provided
	var avatarData *AvatarData
	if params.AvatarPath != "" {
		avatarData, err = loadAvatar(cctx.Context, params.AvatarPath, logger)
		if err != nil {
			return err
		}
	}

	// Load yuge config if provided
	var yugeConfig types.FeedConfig
	if params.YugeConfigPath != "" {
		p, err := provider.NewFileFeedConfigProvider(params.YugeConfigPath)
		if err != nil {
			return err
		}
		yugeConfig = p.FeedConfig()
		logger.Debug("loaded yuge config", "config", func() string {
			if j, err := json.Marshal(yugeConfig); err == nil {
				return string(j)
			}
			return fmt.Errorf("failed to marshal yuge config: %w", err).Error()
		}())
	}

	// Create session
	ctx := cctx.Context
	client, err := createSessionWithClient(ctx, params.Host, params.Identifier.String(), params.Password, logger)
	if err != nil {
		return err
	}
	defer cleanupSessionWithClient(ctx, client, logger)

	// Publish feed using the client interface
	return publishFeedWithClient(ctx, client, params, avatarData, yugeConfig, logger, force)
}

// publishFeedWithClient publishes a feed using the ATProtoClient interface
func publishFeedWithClient(ctx context.Context, client ATProtoClient, params *FeedParams, avatarData *AvatarData, yugeConfig types.FeedConfig, logger *slog.Logger, force bool) error {
	// Check for existing record and get confirmation if needed
	prevCid, err := checkExistingRecordWithClient(ctx, client, params.RecordKey, logger, force)
	if err != nil {
		return err
	}

	if params.DryRun {
		logger.Info("dry-run mode enabled, skipping feed publish")
	} else {
		// Upload avatar if provided
		var blob *lexutil.LexBlob
		if avatarData != nil {
			blob, err = uploadAvatarWithClient(ctx, client, avatarData, logger)
			if err != nil {
				return err
			}
		}

		// Publish feed record
		if err := publishFeedRecordWithClient(ctx, client, params, prevCid, blob, yugeConfig, logger); err != nil {
			return err
		}

		logger.Info("feed publish completed successfully")
	}
	return nil
}

// parseFeedParams parses CLI parameters into FeedParams
func parseFeedParams(cctx *cli.Context) (*FeedParams, error) {
	if cctx.NArg() < 1 {
		return nil, fmt.Errorf("feed record name is required")
	}

	identifier := cctx.String("identifier")
	if identifier == "" {
		var err error
		identifier, err = promptForInput("Bluesky identifier (handle or DID)")
		if err != nil {
			return nil, err
		}
	}

	parsedIdentifier, err := syntax.ParseAtIdentifier(identifier)
	if err != nil {
		return nil, fmt.Errorf("invalid identifier: %w", err)
	}

	password := cctx.String("password")
	if password == "" {
		var err error
		password, err = promptForPassword("Bluesky password")
		if err != nil {
			return nil, err
		}
	}

	return &FeedParams{
		RecordKey:           cctx.Args().Get(0),
		Identifier:          parsedIdentifier,
		Password:            password,
		ServiceDID:          cctx.String("service-did"),
		DisplayName:         cctx.String("display-name"),
		Description:         cctx.String("description"),
		AvatarPath:          cctx.String("avatar"),
		ContentMode:         cctx.String("content-mode"),
		AcceptsInteractions: cctx.Bool("accepts-interactions"),
		YugeConfigPath:      cctx.String("yuge-config"),
		Host:                cctx.String("host"),
		DryRun:              cctx.Bool("dry-run"),
	}, nil
}

// validateFeedParams validates feed parameters
func validateFeedParams(params *FeedParams, logger *slog.Logger) error {
	logger.Debug("validating inputs")

	if params.RecordKey == "" {
		return fmt.Errorf("record key cannot be empty")
	}
	if _, err := syntax.ParseRecordKey(params.RecordKey); err != nil {
		logger.Error("invalid record key", "error", err)
		return fmt.Errorf("invalid record key: %w", err)
	}
	logger.Debug("record key validated", "recordKey", params.RecordKey)

	if params.ServiceDID == "" {
		return fmt.Errorf("service DID cannot be empty")
	}

	if params.DisplayName == "" {
		return fmt.Errorf("display name cannot be empty")
	}
	if len(params.DisplayName) > MAX_DISPLAY_NAME_LENGTH {
		return fmt.Errorf("display name exceeds maximum length of 240 characters (got %d)", len(params.DisplayName))
	}
	logger.Debug("display name validated",
		"displayName", params.DisplayName,
		"length", len(params.DisplayName),
	)

	if len(params.Description) > MAX_DESCRIPTION_LENGTH {
		return fmt.Errorf("description exceeds maximum length of 3000 characters")
	}
	if params.Description != "" {
		logger.Debug("description validated", "length", len(params.Description))
	}

	// Validate content mode
	if err := validateContentMode(params.ContentMode); err != nil {
		return err
	}
	logger.Debug("content mode validated", "contentMode", params.ContentMode)

	if params.YugeConfigPath != "" {
		// Check if file exists
		if _, err := os.Stat(params.YugeConfigPath); os.IsNotExist(err) {
			logger.Error("yuge config file not found", "path", params.YugeConfigPath)
			return fmt.Errorf("yuge config file not found: %s", params.YugeConfigPath)
		}
	}

	return nil
}

// validateContentMode validates and returns the full content mode identifier
func validateContentMode(mode string) error {
	if mode == "" {
		return nil // empty is valid, defaults to unspecified
	}
	switch mode {
	case "unspecified", "video":
		return nil
	default:
		return fmt.Errorf("invalid content mode: %s (must be one of: unspecified, video)", mode)
	}
}

// getFullContentMode returns the full content mode identifier
func getFullContentMode(mode string) string {
	switch mode {
	// "" defaults to unspecified
	case "", "unspecified":
		return CONTENT_MODE_UNSPECIFIED
	case "video":
		return CONTENT_MODE_VIDEO
	default:
		return ""
	}
}

// loadAvatar loads and validates an avatar file
func loadAvatar(ctx context.Context, path string, logger *slog.Logger) (*AvatarData, error) {
	logger.Info("loading avatar", "path", path)

	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logger.Error("avatar file not found", "path", path)
		return nil, fmt.Errorf("avatar file not found: %s", path)
	}
	logger.Debug("avatar file exists")

	// Open and decode image
	file, err := os.Open(path)
	if err != nil {
		logger.Error("failed to open avatar file", "error", err)
		return nil, fmt.Errorf("failed to open avatar file: %w", err)
	}
	defer file.Close()

	// Decode image to verify format
	_, format, err := image.DecodeConfig(file)
	if err != nil {
		logger.Error("failed to decode avatar image", "error", err)
		return nil, fmt.Errorf("failed to decode avatar image: %w", err)
	}

	// Check format (only JPEG and PNG are supported)
	if format != "jpeg" && format != "png" {
		logger.Error("unsupported avatar format", "format", format)
		return nil, fmt.Errorf("unsupported avatar format %s (only JPEG and PNG are supported)", format)
	}
	logger.Debug("avatar format validated", "format", format)

	// Reset file pointer to beginning
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		logger.Error("failed to reset file pointer", "error", err)
		return nil, fmt.Errorf("failed to reset file pointer: %w", err)
	}

	// Read file data
	fileInfo, err := file.Stat()
	if err != nil {
		logger.Error("failed to get avatar file info", "error", err)
		return nil, fmt.Errorf("failed to get avatar file info: %w", err)
	}

	// Check file size (1MB limit)
	if fileInfo.Size() > MAX_AVATAR_SIZE_BYTES {
		logger.Error("avatar file size exceeds limit",
			"size", fileInfo.Size(),
			"limit", MAX_AVATAR_SIZE_BYTES,
		)
		return nil, fmt.Errorf("avatar file size exceeds 1MB limit")
	}
	logger.Debug("avatar size validated",
		"bytes", fileInfo.Size(),
		"kilobytes", float64(fileInfo.Size())/1024,
	)

	data, err := io.ReadAll(file)
	if err != nil {
		logger.Error("failed to read avatar file", "error", err)
		return nil, fmt.Errorf("failed to read avatar file: %w", err)
	}

	mimeType := "image/jpeg"
	if format == "png" {
		mimeType = "image/png"
	}

	logger.Info("avatar loaded successfully",
		"mimeType", mimeType,
		"bytes", len(data),
	)

	return &AvatarData{
		Data:     data,
		MimeType: mimeType,
	}, nil
}

// checkExistingRecordWithClient checks if a feed record already exists and prompts for confirmation
func checkExistingRecordWithClient(ctx context.Context, client ATProtoClient, recordKey string, logger *slog.Logger, force bool) (string, error) {
	logger.Info("checking if feed record already exists", "recordKey", recordKey)

	respgr, err := client.GetRecord(ctx, COLLECTION_TYPE_FEED_GENERATOR, client.GetDID(), recordKey)
	if err != nil {
		if strings.Contains(err.Error(), "400") && strings.Contains(err.Error(), "RecordNotFound") {
			logger.Debug(err.Error())
			logger.Debug("feed record does not exist, proceeding to create", "recordKey", recordKey)
			return "", nil
		}
		logger.Error("failed to fetch feed record", "recordKey", recordKey, "error", err)
		return "", fmt.Errorf("failed to fetch feed record: %w", err)
	}

	prevCid := *respgr.Cid
	logger.Debug("feed record already exists", "recordKey", recordKey, "uri", respgr.Uri, "cid", prevCid)

	if force {
		logger.Info("force flag set, skipping confirmation prompt for overwrite", "recordKey", recordKey)
		return prevCid, nil
	}
	// Prompt to confirm overwrite if exists
	confirmed, err := promptConfirmation(fmt.Sprintf("Feed record with key '%s' already exists. Overwrite?", recordKey))
	if err != nil {
		logger.Warn("failed to read user input", "error", err)
		return "", err
	}

	if !confirmed {
		logger.Info("aborting feed publish as per user request", "recordKey", recordKey)
		fmt.Println("Aborted feed publish.")
		return "", fmt.Errorf("user cancelled operation")
	}

	logger.Info("user confirmed to overwrite existing feed record", "recordKey", recordKey)
	return prevCid, nil
}

// uploadAvatarWithClient uploads avatar data to the PDS
func uploadAvatarWithClient(ctx context.Context, client ATProtoClient, avatar *AvatarData, logger *slog.Logger) (*lexutil.LexBlob, error) {
	logger.Info("uploading avatar to PDS",
		"bytes", len(avatar.Data),
		"mimeType", avatar.MimeType,
	)

	resp, err := client.UploadBlob(ctx, bytes.NewReader(avatar.Data))
	if err != nil {
		logger.Error("failed to upload avatar blob", "error", err)
		return nil, fmt.Errorf("failed to upload avatar: %w", err)
	}

	logger.Info("avatar uploaded successfully", "blobRef", resp.Blob.Ref)
	return resp.Blob, nil
}

// publishFeedRecordWithClient publishes the feed record to the PDS
func publishFeedRecordWithClient(ctx context.Context, client ATProtoClient, params *FeedParams, prevCid string, blob *lexutil.LexBlob, yugeConfig types.FeedConfig, logger *slog.Logger) error {
	fullContentMode := getFullContentMode(params.ContentMode)

	// Set up feed record
	r := comAtprotoFeedGeneratorWithYugeFeed{
		FeedGenerator: bsky.FeedGenerator{
			LexiconTypeID:       COLLECTION_TYPE_FEED_GENERATOR,
			DisplayName:         params.DisplayName,
			Description:         &params.Description,
			Did:                 params.ServiceDID,
			CreatedAt:           time.Now().Format(time.RFC3339),
			Avatar:              blob,
			ContentMode:         &fullContentMode,
			AcceptsInteractions: &params.AcceptsInteractions,
		},
		YugeFeed: yugeConfig, // Set to nil or load from config if needed
	}
	recordJSON, _ := json.Marshal(r)
	logger.Debug("constructed feed record", "record", string(recordJSON))

	i := &RepoPutRecordWithRawRecord_Input{
		RepoPutRecord_Input: atproto.RepoPutRecord_Input{
			Rkey:       params.RecordKey,
			Collection: COLLECTION_TYPE_FEED_GENERATOR,
			Repo:       client.GetDID(),
			Validate:   func() *bool { v := true; return &v }(),
			SwapRecord: func() *string {
				if prevCid != "" {
					return &prevCid
				}
				return nil
			}(),
		},
		Record: func() json.RawMessage {
			recordJSON, _ := json.Marshal(r)
			return recordJSON
		}(),
	}

	// Log record JSON
	if recordJSON, err := json.Marshal(i.Record); err == nil {
		logger.Info("publishing feed record to PDS", "recordJSON", string(recordJSON))
	} else {
		logger.Info("publishing feed record to PDS", "record", i.Record)
	}

	resp, err := client.PutRecord(ctx, i)
	if err != nil {
		logger.Error("failed to publish record", "error", err)
		return fmt.Errorf("failed to publish record: %w", err)
	}

	logger.Info("feed record published successfully",
		"uri", resp.Uri,
		"cid", resp.Cid,
	)

	return nil
}
