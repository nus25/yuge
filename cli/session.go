package cli

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/bluesky-social/indigo/api/atproto"
)

// createSessionWithClient creates an authenticated client session
func createSessionWithClient(ctx context.Context, host, identifier, password string, logger *slog.Logger) (ATProtoClient, error) {
	logger.Debug("creating session", "host", host, "identifier", identifier)

	client := NewXRPCClientWrapper(host)

	input := &atproto.ServerCreateSession_Input{
		Identifier: identifier,
		Password:   password,
	}

	output, err := client.CreateSession(ctx, input)
	if err != nil {
		logger.Error("failed to create session", "error", err)
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	logger.Debug("session created successfully",
		"did", output.Did,
		"handle", output.Handle,
	)

	return client, nil
}

// cleanupSessionWithClient deletes the session
func cleanupSessionWithClient(ctx context.Context, client ATProtoClient, logger *slog.Logger) {
	if err := client.DeleteSession(ctx); err != nil {
		logger.Warn("failed to delete session", "error", err)
	}
}
