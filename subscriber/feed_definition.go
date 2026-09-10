package subscriber

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

var _ FeedDefinitionProvider = (*FileFeedDefinitionProvider)(nil) //type check

const FILE_NAME = "feedlist.yaml"

type FeedDefinitionProvider interface {
	GetFeedDefinition(feedId string) (FeedDefinition, error)
	GetFeedDefinitionList() (*FeedDefinitionList, error)
}

type FeedDefinition struct {
	ID            string `yaml:"id" json:"id"`
	URI           string `yaml:"uri" json:"uri"`
	ConfigFile    string `yaml:"configFile,omitempty" json:"configFile,omitempty"`
	InactiveStart string `yaml:"inactiveStart,omitempty" json:"inactiveStart,omitempty"`
}

type FeedDefinitionList struct {
	Feeds []FeedDefinition `yaml:"feeds" json:"feeds"`
}

// FileFeedDefinitionProvider loads initial feed definitions from YAML.
type FileFeedDefinitionProvider struct {
	baseDir string
}

func NewFileFeedDefinitionProvider(dir string) (*FileFeedDefinitionProvider, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create feed definition directory: %w", err)
	}
	return &FileFeedDefinitionProvider{
		baseDir: dir,
	}, nil
}

func (p *FileFeedDefinitionProvider) GetFeedDefinition(feedId string) (FeedDefinition, error) {
	list, err := p.GetFeedDefinitionList()
	if err != nil {
		return FeedDefinition{}, err
	}

	for _, def := range list.Feeds {
		if def.ID == feedId {
			return def, nil
		}
	}

	return FeedDefinition{}, fmt.Errorf("feed definition not found: %s", feedId)
}

func (p *FileFeedDefinitionProvider) GetFeedDefinitionList() (*FeedDefinitionList, error) {
	// パスの検証
	if _, err := os.Stat(p.baseDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory not found: %s", p.baseDir)
	}

	feedListPath := filepath.Join(p.baseDir, FILE_NAME)
	if _, err := os.Stat(feedListPath); os.IsNotExist(err) {
		return &FeedDefinitionList{Feeds: []FeedDefinition{}}, nil
	}
	data, err := os.ReadFile(feedListPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read feed list file: %w", err)
	}
	var list FeedDefinitionList
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to parse feed list yaml: %w", err)
	}

	return &list, nil
}

func (p *FileFeedDefinitionProvider) AddFeedDefinition(def FeedDefinition) error {
	list, err := p.GetFeedDefinitionList()
	if err != nil {
		return fmt.Errorf("get feed definition list: %w", err)
	}
	for _, existing := range list.Feeds {
		if existing.ID == def.ID {
			return fmt.Errorf("feed already exists: %s", def.ID)
		}
	}
	list.Feeds = append(list.Feeds, def)
	data, err := yaml.Marshal(list)
	if err != nil {
		return fmt.Errorf("marshal feed definition list: %w", err)
	}
	if err := os.WriteFile(filepath.Join(p.baseDir, FILE_NAME), data, 0644); err != nil {
		return fmt.Errorf("write feed list file: %w", err)
	}
	return nil
}
