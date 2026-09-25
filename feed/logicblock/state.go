package logicblock

import "context"

// StateRepository persists state owned by a logic block.
type StateRepository interface {
	LoadLogicBlockState(ctx context.Context, feedID string, blockKey string) (state []byte, found bool, err error)
	SaveLogicBlockState(ctx context.Context, feedID string, blockKey string, version int, state []byte) error
}

// StatefulLogicBlock restores its own state and persists subsequent mutations.
type StatefulLogicBlock interface {
	RestoreState(ctx context.Context, feedID string, blockKey string, repository StateRepository) error
}
