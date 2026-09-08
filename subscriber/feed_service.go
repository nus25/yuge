package subscriber

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/nus25/yuge/feed"
	"github.com/nus25/yuge/feed/config/provider"
	storepkg "github.com/nus25/yuge/feed/store"
	"github.com/nus25/yuge/types"
	"golang.org/x/sync/errgroup"
)

const runtimeStatusFilename = "status.json"

type runtimeFeedState struct {
	Definition FeedDefinition `json:"definition"`
	Status     FeedStatus     `json:"status"`
}

type runtimeFeedSnapshot struct {
	Feeds []runtimeFeedState `json:"feeds"`
}

type FeedService struct {
	definitionProvider  FeedDefinitionProvider
	configDir           string
	dataDir             string
	storeLoader         storepkg.PostLoader
	mutationCoordinator PostMutationCoordinator
	feeds               map[string]FeedInfo
	logger              *slog.Logger
	mu                  sync.RWMutex
	statusMu            sync.Mutex
	feedOpMu            sync.Mutex
	feedOpLocks         map[string]*sync.Mutex
}

func NewFeedService(configDir string, dataDir string, definitionProvider FeedDefinitionProvider, logger *slog.Logger) (*FeedService, error) {
	if logger != nil {
		logger = slog.Default()
	}
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}
	}
	if definitionProvider == nil {
		logger.Warn("no definition provider specified")
	}
	return &FeedService{
		configDir:          configDir,
		dataDir:            dataDir,
		definitionProvider: definitionProvider,
		feeds:              make(map[string]FeedInfo),
		feedOpLocks:        make(map[string]*sync.Mutex),
		logger:             logger,
	}, nil
}

func (s *FeedService) withFeedOperationLock(feedID string, fn func() error) error {
	lock := s.getFeedOperationLock(feedID)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

func (s *FeedService) resolveStoreResources() storepkg.PostLoader {
	s.mu.RLock()
	loader := s.storeLoader
	s.mu.RUnlock()
	return loader
}

func (s *FeedService) getFeedOperationLock(feedID string) *sync.Mutex {
	s.feedOpMu.Lock()
	defer s.feedOpMu.Unlock()
	if s.feedOpLocks == nil {
		s.feedOpLocks = make(map[string]*sync.Mutex)
	}
	lock, exists := s.feedOpLocks[feedID]
	if !exists {
		lock = &sync.Mutex{}
		s.feedOpLocks[feedID] = lock
	}
	return lock
}

func (s *FeedService) SetStoreLoader(loader storepkg.PostLoader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storeLoader = loader
}

func (s *FeedService) SetMutationCoordinator(coordinator PostMutationCoordinator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mutationCoordinator = coordinator
}

func (s *FeedService) LoadFeeds(ctx context.Context) error {
	snapshot, found, err := s.loadRuntimeSnapshot()
	if err != nil {
		return err
	}
	if !found {
		if s.definitionProvider == nil {
			return fmt.Errorf("feed definition provider is nil")
		}
		fdl, err := s.definitionProvider.GetFeedDefinitionList()
		if err != nil {
			return fmt.Errorf("failed to get feed definition list: %w", err)
		}
		snapshot.Feeds = make([]runtimeFeedState, 0, len(fdl.Feeds))
		for _, def := range fdl.Feeds {
			status := FeedStatusActive
			if def.InactiveStart == "true" {
				status = FeedStatusInactive
			}
			snapshot.Feeds = append(snapshot.Feeds, runtimeFeedState{Definition: def, Status: FeedStatus{FeedID: def.ID, LastStatus: status, LastUpdated: time.Now()}})
		}
	}

	s.mu.RLock()
	currentFeeds := make(map[string]bool, len(s.feeds))
	for id := range s.feeds {
		currentFeeds[id] = true
	}
	s.mu.RUnlock()

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10) // Limit the number of concurrent executions

	for _, state := range snapshot.Feeds {
		state := state
		g.Go(func() error {
			def := state.Definition
			_, exists := s.GetFeedInfo(def.ID)

			if exists {
				s.logger.Info("updating existing feed",
					slog.String("feed_id", def.ID),
					slog.String("operation", "update"))
				if err := s.ReloadFeed(ctx, def.ID); err != nil {
					return fmt.Errorf("failed to update feed %s: %w", def.ID, err)
				}
			} else {
				initialStatus := state.Status.LastStatus
				if initialStatus == FeedStatusError || initialStatus == FeedStatusUnknown {
					initialStatus = FeedStatusActive
				}
				if err := s.CreateFeed(ctx, def, initialStatus); err != nil {
					return fmt.Errorf("failed to create feed %s: %w", def.ID, err)
				}
			}

			s.mu.Lock()
			delete(currentFeeds, def.ID)
			s.mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// delete unnecessary feeds
	for id := range currentFeeds {
		if err := s.DeleteFeed(id); err != nil {
			s.logger.Error("failed to delete feed",
				slog.String("feed_id", id),
				slog.String("error", err.Error()))
		}
	}

	return s.saveRuntimeSnapshot()
}

func (s *FeedService) ReloadFeed(ctx context.Context, feedId string) error {
	return s.withFeedOperationLock(feedId, func() error {
		return s.reloadFeed(ctx, feedId)
	})
}

func (s *FeedService) reloadFeed(ctx context.Context, feedId string) error {
	s.logger.Info("reloading feed", "feedId", feedId)

	// get existing feed
	fi, exists := s.GetFeedInfo(feedId)
	if !exists {
		return fmt.Errorf("feed %s not found", feedId)
	}

	def := fi.Definition

	// shutdown existing feed
	if fi.Feed != nil {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := fi.Feed.Shutdown(ctx); err != nil {
			s.logger.Error("failed to shutdown existing feed", "feedId", feedId, "error", err)
			// even if shutdown fails, continue processing
		}
	}

	// delete from feedlist
	s.unregisterFeed(feedId)
	var newStatus Status
	if fi.Status.LastStatus == FeedStatusInactive {
		//inactive
		newStatus = FeedStatusInactive
	} else {
		//error,active
		newStatus = FeedStatusActive
	}

	// create new feed
	if err := s.createFeed(ctx, def, newStatus); err != nil {
		if saveErr := s.saveRuntimeSnapshot(); saveErr != nil {
			return fmt.Errorf("failed to create new feed: %w; save runtime status: %v", err, saveErr)
		}
		return fmt.Errorf("failed to create new feed: %w", err)
	}
	if err := s.saveRuntimeSnapshot(); err != nil {
		return fmt.Errorf("save runtime status: %w", err)
	}

	s.logger.Info("feed reloaded successfully", "feedId", feedId)
	return nil
}

func (s *FeedService) ClearFeed(ctx context.Context, feedId string) error {
	return s.withFeedOperationLock(feedId, func() error {
		return s.clearFeed(ctx, feedId)
	})
}

func (s *FeedService) clearFeed(ctx context.Context, feedId string) error {
	fi, exists := s.GetFeedInfo(feedId)
	if !exists {
		return fmt.Errorf("feed %s not found", feedId)
	}
	if fi.Feed == nil {
		return fmt.Errorf("feed %s is not initialized", feedId)
	}

	s.mu.RLock()
	storeLoader := s.storeLoader
	mutationCoordinator := s.mutationCoordinator
	s.mu.RUnlock()

	if storeLoader != nil {
		if mutationCoordinator == nil {
			return fmt.Errorf("post mutation coordinator is required to clear loader-backed feed %s", feedId)
		}
		if err := mutationCoordinator.ClearFeed(ctx, ClearFeedParams{
			FeedID:  feedId,
			FeedURI: types.FeedUri(fi.Definition.URI),
		}); err != nil {
			return fmt.Errorf("clear persisted posts during clear feed: %w", err)
		}
	}

	if err := fi.Feed.Clear(); err != nil {
		return fmt.Errorf("clear feed: %w", err)
	}
	return nil
}

func (s *FeedService) TrimFeed(ctx context.Context, feedId string, remain int) error {
	return s.withFeedOperationLock(feedId, func() error {
		return s.trimFeed(ctx, feedId, remain)
	})
}

func (s *FeedService) trimFeed(ctx context.Context, feedId string, remain int) error {
	if remain < 0 {
		return fmt.Errorf("remain must be >= 0")
	}

	fi, exists := s.GetFeedInfo(feedId)
	if !exists {
		return fmt.Errorf("feed %s not found", feedId)
	}
	if fi.Feed == nil {
		return fmt.Errorf("feed %s is not initialized", feedId)
	}

	s.mu.RLock()
	storeLoader := s.storeLoader
	mutationCoordinator := s.mutationCoordinator
	s.mu.RUnlock()

	if storeLoader != nil {
		if mutationCoordinator == nil {
			return fmt.Errorf("post mutation coordinator is required to trim loader-backed feed %s", feedId)
		}
		if err := mutationCoordinator.TrimFeed(ctx, TrimFeedParams{
			FeedID:  feedId,
			FeedURI: types.FeedUri(fi.Definition.URI),
			Remain:  remain,
		}); err != nil {
			return fmt.Errorf("trim persisted posts during trim feed: %w", err)
		}
	}

	if err := fi.Feed.Trim(remain); err != nil {
		return fmt.Errorf("trim feed: %w", err)
	}
	return nil
}

func (s *FeedService) Shutdown(ctx context.Context) error {
	var mu sync.Mutex
	var errs []error
	var wg sync.WaitGroup

	for _, fi := range s.feeds {
		if fi.Feed != nil {
			wg.Add(1)
			go func(feed feed.Feed) {
				defer wg.Done()
				if err := feed.Shutdown(ctx); err != nil {
					s.logger.Error("failed to shutdown feed",
						"feedId", feed.FeedId(),
						"error", err)

					mu.Lock()
					errs = append(errs, fmt.Errorf("feed %s: %w", feed.FeedId(), err))
					mu.Unlock()
				}
			}(fi.Feed)
		}
	}

	wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("multiple feeds failed to shutdown: %v", errs)
	}
	return nil
}

func (s *FeedService) CreateFeed(ctx context.Context, def FeedDefinition, status Status) (err error) {
	return s.withFeedOperationLock(def.ID, func() error {
		createErr := s.createFeed(ctx, def, status)
		saveErr := s.saveRuntimeSnapshot()
		if createErr != nil {
			if saveErr != nil {
				return fmt.Errorf("create feed: %w; save runtime status: %v", createErr, saveErr)
			}
			return createErr
		}
		return saveErr
	})
}

func (s *FeedService) createFeed(ctx context.Context, def FeedDefinition, status Status) (err error) {
	feedId := def.ID
	configFile := def.ConfigFile
	feedUri := def.URI
	s.logger.Info("📃creating feed", "feedId", feedId, "feedUri", feedUri, "configPath", configFile)

	_, exists := s.GetFeedInfo(feedId)
	if exists {
		return fmt.Errorf("feed %s already exists", feedId)
	}

	feedStatus := FeedStatus{
		FeedID:      feedId,
		LastStatus:  status,
		LastUpdated: time.Now(),
		Error:       "",
	}
	defer func() {
		//if failed to create feed, set error log
		if err != nil {
			feedStatus.SetError(err)
			s.registerFeed(def, nil, feedStatus)
		}
	}()

	// load feedConfig
	var cp provider.FeedConfigProvider
	if s.configDir != "" && configFile != "" {
		// load from file
		path := filepath.Join(s.configDir, configFile)
		var err error
		cp, err = provider.NewFileFeedConfigProvider(path)
		if err != nil {
			return fmt.Errorf("failed to create feed config: %w", err)
		}
	} else {
		// if no file specified, get config from PDS
		cp, err = provider.NewPDSFeedConfigProvider(feedUri)
		if err != nil {
			return fmt.Errorf("failed to create feed config: %w", err)
		}
	}

	//feed
	storeLoader := s.resolveStoreResources()
	initctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	newFeed, err := feed.NewFeedWithOptions(initctx, feedId, feedUri, feed.FeedOptions{
		Config:      cp.FeedConfig(),
		StoreLoader: storeLoader,
		Logger:      s.logger,
	})

	if err != nil {
		return fmt.Errorf("failed to create feed: %w", err)
	} else {
		s.logger.Info("success to create feed", "feedId", feedId)
	}
	s.registerFeed(def, newFeed, feedStatus)
	return nil
}

func (s *FeedService) DeleteFeed(feedId string) error {
	return s.withFeedOperationLock(feedId, func() error {
		return s.deleteFeed(feedId)
	})
}

func (s *FeedService) deleteFeed(feedId string) error {
	s.mu.Lock()
	fi, exists := s.feeds[feedId]
	s.mu.Unlock()

	if !exists {
		// if already deleted, treat as success
		s.logger.Info("feed already deleted", "feedId", feedId)
		return nil
	}

	// shutdown feed
	if fi.Feed != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fi.Feed.Shutdown(ctx); err != nil {
			s.logger.Error("failed to shutdown feed", "feedId", feedId, "error", err)
			// even if shutdown fails, continue deleting
		}
	}

	// delete from service
	s.unregisterFeed(feedId)

	return s.saveRuntimeSnapshot()
}

func (s *FeedService) registerFeed(def FeedDefinition, feed feed.Feed, status FeedStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Info("adding new feed", "feedId", def.ID)
	s.feeds[def.ID] = FeedInfo{Definition: def, Feed: feed, Status: status}
}

func (s *FeedService) unregisterFeed(feedId string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.feeds[feedId]; !exists {
		s.logger.Info("feed not found", "feedId", feedId)
		return
	}
	s.logger.Info("deleting feed", "feedId", feedId)
	delete(s.feeds, feedId)
}

func (s *FeedService) UpdateStatus(feedId string, status Status) error {
	s.mu.Lock()

	fi, exists := s.feeds[feedId]
	if !exists {
		s.mu.Unlock()
		return fmt.Errorf("feed not found: %s", feedId)
	}
	fi.Status.LastStatus = status
	fi.Status.LastUpdated = time.Now()
	s.feeds[feedId] = fi
	s.mu.Unlock()
	if err := s.saveRuntimeSnapshot(); err != nil {
		return fmt.Errorf("save runtime status: %w", err)
	}
	s.logger.Info("feed status updated", "feedId", feedId, "status", fi.Status.LastStatus)
	return nil
}

func (s *FeedService) UpdateFeed(ctx context.Context, def FeedDefinition, status Status) error {
	return s.withFeedOperationLock(def.ID, func() error {
		fi, exists := s.GetFeedInfo(def.ID)
		if !exists {
			return fmt.Errorf("feed %s not found", def.ID)
		}
		if fi.Feed != nil {
			if err := fi.Feed.Shutdown(ctx); err != nil {
				s.logger.Error("failed to shutdown existing feed", "feedId", def.ID, "error", err)
			}
		}
		s.unregisterFeed(def.ID)
		createErr := s.createFeed(ctx, def, status)
		saveErr := s.saveRuntimeSnapshot()
		if createErr != nil {
			if saveErr != nil {
				return fmt.Errorf("create updated feed: %w; save runtime status: %v", createErr, saveErr)
			}
			return fmt.Errorf("create updated feed: %w", createErr)
		}
		return saveErr
	})
}

func (s *FeedService) runtimeStatusPath() string {
	return filepath.Join(s.dataDir, runtimeStatusFilename)
}

func (s *FeedService) loadRuntimeSnapshot() (runtimeFeedSnapshot, bool, error) {
	data, err := os.ReadFile(s.runtimeStatusPath())
	if err != nil {
		if os.IsNotExist(err) {
			return runtimeFeedSnapshot{}, false, nil
		}
		return runtimeFeedSnapshot{}, false, fmt.Errorf("read runtime status: %w", err)
	}
	var snapshot runtimeFeedSnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return runtimeFeedSnapshot{}, false, fmt.Errorf("parse runtime status: %w", err)
	}
	return snapshot, true, nil
}

func (s *FeedService) saveRuntimeSnapshot() error {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()
	s.mu.RLock()
	snapshot := runtimeFeedSnapshot{Feeds: make([]runtimeFeedState, 0, len(s.feeds))}
	for _, info := range s.feeds {
		snapshot.Feeds = append(snapshot.Feeds, runtimeFeedState{Definition: info.Definition, Status: info.Status})
	}
	s.mu.RUnlock()
	sort.Slice(snapshot.Feeds, func(i, j int) bool { return snapshot.Feeds[i].Definition.ID < snapshot.Feeds[j].Definition.ID })
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal runtime status: %w", err)
	}
	temporaryPath := s.runtimeStatusPath() + ".tmp"
	if err := os.WriteFile(temporaryPath, data, 0644); err != nil {
		return fmt.Errorf("write runtime status: %w", err)
	}
	if err := os.Rename(temporaryPath, s.runtimeStatusPath()); err != nil {
		return fmt.Errorf("replace runtime status: %w", err)
	}
	return nil
}

func (s *FeedService) GetFeedStatus(feedId string) (status FeedStatus, exists bool) {
	fi, exists := s.GetFeedInfo(feedId)
	if !exists {
		return FeedStatus{}, false
	}
	return fi.Status, true
}

func (s *FeedService) GetActiveFeedIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	feedIds := make([]string, 0)
	for id, f := range s.feeds {
		if f.Status.LastStatus != FeedStatusError {
			feedIds = append(feedIds, id)
		}
	}
	return feedIds
}

func (s *FeedService) GetAllFeeds() map[string]FeedInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	feeds := make(map[string]FeedInfo, len(s.feeds))
	for id, info := range s.feeds {
		feeds[id] = info
	}
	return feeds
}

func (s *FeedService) GetFeedInfo(feedId string) (info *FeedInfo, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if fi, ok := s.feeds[feedId]; ok {
		return &fi, true
	}
	return nil, false
}
