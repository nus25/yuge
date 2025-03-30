package subscriber

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nus25/yuge/feed"
	"github.com/nus25/yuge/feed/config/provider"
	"github.com/nus25/yuge/feed/store/editor"
	"golang.org/x/sync/errgroup"
)

type FeedService struct {
	definitionProvider FeedDefinitionProvider
	configDir          string
	dataDir            string
	storeEditor        editor.StoreEditor
	feeds              map[string]FeedInfo
	logger             *slog.Logger
	mu                 sync.RWMutex
}

func NewFeedService(configDir string, dataDir string, definitionProvider FeedDefinitionProvider, storeEditor editor.StoreEditor, logger *slog.Logger) (*FeedService, error) {
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
	// use file editor if not specified
	if storeEditor == nil {
		var err error
		storeEditor, err = editor.NewFileEditor(dataDir, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create file editor: %w", err)
		}
	}
	return &FeedService{
		configDir:          configDir,
		dataDir:            dataDir,
		definitionProvider: definitionProvider,
		storeEditor:        storeEditor,
		feeds:              make(map[string]FeedInfo),
		logger:             logger,
	}, nil
}

func (s *FeedService) LoadFeeds(ctx context.Context) error {
	if s.definitionProvider == nil {
		return fmt.Errorf("feed definition provider is nil")
	}
	fdl, err := s.definitionProvider.GetFeedDefinitionList()
	if err != nil {
		return fmt.Errorf("failed to get feed definition list: %w", err)
	}

	currentFeeds := make(map[string]bool)
	for id := range s.feeds {
		currentFeeds[id] = true
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10) // Limit the number of concurrent executions

	for _, f := range fdl.Feeds {
		def := f // capture loop variable
		g.Go(func() error {
			_, exists := s.GetFeedInfo(def.ID)

			if exists {
				s.logger.Info("updating existing feed",
					slog.String("feed_id", def.ID),
					slog.String("operation", "update"))
				if err := s.ReloadFeed(ctx, def.ID); err != nil {
					return fmt.Errorf("failed to update feed %s: %w", def.ID, err)
				}
			} else {
				var initialStatus Status
				if def.InactiveStart == "true" {
					initialStatus = FeedStatusInactive
				} else {
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

	return nil
}

func (s *FeedService) ReloadFeed(ctx context.Context, feedId string) error {
	s.logger.Info("reloading feed", "feedId", feedId)

	// get existing feed
	fi, exists := s.GetFeedInfo(feedId)
	if !exists {
		return fmt.Errorf("feed %s not found", feedId)
	}

	// read feed definition list
	def, err := s.definitionProvider.GetFeedDefinition(feedId)
	if err != nil {
		return fmt.Errorf("failed to get feed definition: %w", err)
	}

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
	if err := s.CreateFeed(ctx, def, newStatus); err != nil {
		return fmt.Errorf("failed to create new feed: %w", err)
	}

	s.logger.Info("feed reloaded successfully", "feedId", feedId)
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

	// close store editor
	if err := s.storeEditor.Close(ctx); err != nil {
		return fmt.Errorf("failed to close store editor: %w", err)
	}

	return nil
}

func (s *FeedService) CreateFeed(ctx context.Context, def FeedDefinition, status Status) (err error) {
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
	initctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	newFeed, err := feed.NewFeedWithOptions(initctx, feedId, feedUri, feed.FeedOptions{
		Config:      cp.FeedConfig(),
		StoreEditor: s.storeEditor,
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

	// delete from definition provider
	if s.definitionProvider != nil {
		if err := s.definitionProvider.DeleteFeedDefinition(feedId); err != nil {
			s.logger.Error("failed to delete feed definition", "feedId", feedId, "error", err)
			return fmt.Errorf("failed to delete feed definition: %w", err)
		}
	}

	return nil
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
	defer s.mu.Unlock()

	fi, exists := s.feeds[feedId]
	if !exists {
		return fmt.Errorf("feed not found: %s", feedId)
	}
	fi.Status.LastStatus = status
	fi.Status.LastUpdated = time.Now()
	s.feeds[feedId] = fi
	s.logger.Info("feed status updated", "feedId", feedId, "status", fi.Status.LastStatus)
	return nil
}

func (s *FeedService) GetFeedStatus(feedId string) (status FeedStatus, exists bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
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
	return s.feeds
}

func (s *FeedService) GetFeedInfo(feedId string) (info *FeedInfo, exists bool) {
	if fi, ok := s.feeds[feedId]; ok {
		return &fi, true
	}
	return nil, false
}
