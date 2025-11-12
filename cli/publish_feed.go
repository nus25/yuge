package cli

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/api/bsky"
	"github.com/bluesky-social/indigo/atproto/syntax"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/urfave/cli/v2"
)

func PublishFeed(cctx *cli.Context) error {
	// Setup logger based on debug flag
	debug := cctx.Bool("debug")
	logLevel := slog.LevelInfo
	if debug {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	if cctx.NArg() < 1 {
		return fmt.Errorf("feed record name is required")
	}
	recordKey := cctx.Args().Get(0)
	identifier := cctx.String("identifier")
	password := cctx.String("password")
	if identifier == "" {
		return fmt.Errorf("identifier is required")
	}
	if password == "" {
		return fmt.Errorf("password is required")
	}

	logger.Debug("input parameters",
		"identifier", identifier,
		"recordKey", recordKey,
		"serviceDID", cctx.String("service-did"),
		"displayName", cctx.String("display-name"),
		"description", cctx.String("description"),
		"avatarPath", cctx.String("avatar"),
		"contentMode", cctx.String("content-mode"),
		"acceptsInteractions", cctx.Bool("accepts-interactions"),
		"host", cctx.String("host"),
	)

	// validate each inputs
	logger.Debug("validating inputs")

	if recordKey == "" {
		return fmt.Errorf("record key cannot be empty")
	}
	_, err := syntax.ParseRecordKey(recordKey)
	if err != nil {
		logger.Error("invalid record key", "error", err)
		return fmt.Errorf("invalid record key: %w", err)
	}
	logger.Debug("record key validated", "recordKey", recordKey)

	if cctx.String("service-did") == "" {
		return fmt.Errorf("service DID cannot be empty")
	}
	logger.Debug("service DID validated", "serviceDID", cctx.String("service-did"))

	if cctx.String("display-name") == "" {
		return fmt.Errorf("display name cannot be empty")
	}
	if len(cctx.String("display-name")) > 240 {
		return fmt.Errorf("display name exceeds maximum length of 240 characters")
	}
	logger.Debug("display name validated",
		"displayName", cctx.String("display-name"),
		"length", len(cctx.String("display-name")),
	)

	if len(cctx.String("description")) > 3000 {
		return fmt.Errorf("description exceeds maximum length of 3000 characters")
	}
	if cctx.String("description") != "" {
		logger.Debug("description validated", "length", len(cctx.String("description")))
	}

	contentMode := cctx.String("content-mode")
	if contentMode != "unspecified" && contentMode != "video" {
		return fmt.Errorf("invalid content mode: %s", contentMode)
	}
	logger.Debug("content mode validated", "contentMode", contentMode)

	// load avatar file if provided
	avatarPath := cctx.String("avatar")
	var avatarData []byte
	var avatarMimeType string

	if avatarPath != "" {
		logger.Info("loading avatar", "path", avatarPath)

		// Check if file exists
		if _, err := os.Stat(avatarPath); os.IsNotExist(err) {
			logger.Error("avatar file not found", "path", avatarPath)
			return fmt.Errorf("avatar file not found: %s", avatarPath)
		}
		logger.Debug("avatar file exists")

		// Open and decode image
		file, err := os.Open(avatarPath)
		if err != nil {
			logger.Error("failed to open avatar file", "error", err)
			return fmt.Errorf("failed to open avatar file: %w", err)
		}
		defer file.Close()

		// Decode image to verify format
		_, format, err := image.DecodeConfig(file)
		if err != nil {
			logger.Error("failed to decode avatar image", "error", err)
			return fmt.Errorf("failed to decode avatar image: %w", err)
		}

		// Check format (only JPEG and PNG are supported)
		if format != "jpeg" && format != "png" {
			logger.Error("unsupported avatar format", "format", format)
			return fmt.Errorf("unsupported avatar format %s (only JPEG and PNG are supported)", format)
		}
		logger.Debug("avatar format validated", "format", format)

		// Reset file pointer to beginning
		file.Seek(0, 0)

		// Read file data
		fileInfo, err := file.Stat()
		if err != nil {
			logger.Error("failed to get avatar file info", "error", err)
			return fmt.Errorf("failed to get avatar file info: %w", err)
		}

		// Check file size (1MB limit)
		const maxSize = 1 * 1024 * 1024 // 1MB
		if fileInfo.Size() > maxSize {
			logger.Error("avatar file size exceeds limit",
				"size", fileInfo.Size(),
				"limit", maxSize,
			)
			return fmt.Errorf("avatar file size exceeds 1MB limit")
		}
		logger.Debug("avatar size validated",
			"bytes", fileInfo.Size(),
			"kilobytes", float64(fileInfo.Size())/1024,
		)

		avatarData = make([]byte, fileInfo.Size())
		if _, err := file.Read(avatarData); err != nil {
			logger.Error("failed to read avatar file", "error", err)
			return fmt.Errorf("failed to read avatar file: %w", err)
		}

		if strings.HasSuffix(avatarPath, ".png") {
			avatarMimeType = "image/png"
		} else {
			avatarMimeType = "image/jpeg"
		}

		logger.Info("avatar loaded successfully",
			"mimeType", avatarMimeType,
			"bytes", len(avatarData),
		)
	}

	// Create atproto client
	logger.Info("creating session", "host", cctx.String("host"), "identifier", identifier)

	ctx := cctx.Context
	c := &xrpc.Client{
		Host: cctx.String("host"),
	}
	input := &atproto.ServerCreateSession_Input{
		Identifier: identifier,
		Password:   password,
	}
	output, err := atproto.ServerCreateSession(ctx, c, input)
	if err != nil {
		logger.Error("failed to create session", "error", err)
		return fmt.Errorf("failed to create session: %w", err)
	}
	c.Auth = &xrpc.AuthInfo{
		AccessJwt:  output.AccessJwt,
		RefreshJwt: output.RefreshJwt,
		Handle:     output.Handle,
		Did:        output.Did,
	}

	logger.Info("session created successfully",
		"did", output.Did,
		"handle", output.Handle,
	)

	// Set up feed record
	logger.Debug("creating feed record",
		"collection", "app.bsky.feed.generator",
		"repo", c.Auth.Did,
		"recordKey", recordKey,
	)

	r := &atproto.RepoCreateRecord_Input{
		Rkey:       &recordKey,
		Collection: "app.bsky.feed.generator",
		Repo:       c.Auth.Did,
		Record: &lexutil.LexiconTypeDecoder{
			Val: &bsky.FeedGenerator{
				Did:         cctx.String("service-did"),
				DisplayName: cctx.String("display-name"),
				CreatedAt:   time.Now().UTC().Format(time.RFC3339),
				Description: func() *string {
					if desc := cctx.String("description"); desc != "" {
						return &desc
					}
					return nil
				}(),
				ContentMode: func() *string {
					if mode := cctx.String("content-mode"); mode != "unspecified" {
						return &mode
					}
					return nil
				}(),
				AcceptsInteractions: func() *bool {
					accepts := cctx.Bool("accepts-interactions")
					return &accepts
				}(),
				Avatar: func() *lexutil.LexBlob {
					if len(avatarData) > 0 {
						return &lexutil.LexBlob{
							Ref:      lexutil.LexLink{},
							Size:     int64(len(avatarData)),
							MimeType: avatarMimeType,
						}
					}
					return nil
				}(),
			},
		},
	}

	// Publish feed record to PDS
	logger.Info("publishing feed record to PDS")

	resp, err := atproto.RepoCreateRecord(ctx, c, r)
	if err != nil {
		logger.Error("failed to create record", "error", err)
		return err
	}

	logger.Info("feed record created successfully",
		"uri", resp.Uri,
		"cid", resp.Cid,
	)

	fmt.Printf("Feed %s added successfully.\n", recordKey)
	return nil
}
