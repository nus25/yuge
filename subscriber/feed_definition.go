package subscriber

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
)

var _ FeedDefinitionProvider = (*FileFeedDefinitionProvider)(nil) //type check

const FILE_NAME = "feedlist.yaml"

type FeedDefinitionProvider interface {
	GetFeedDefinition(feedId string) (FeedDefinition, error)
	GetFeedDefinitionList() (*FeedDefinitionList, error)
	AddFeedDefinition(def FeedDefinition) error
	UpdateFeedDefinition(def FeedDefinition) error
	DeleteFeedDefinition(feedId string) error
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

// FileFeedDefinitionProvider manages feed definitions in YAML file
// When feed definitions are modified (add/update/delete), saves new version as:
// baseDir/version/configname_v1_YYYYMMDD_hhmmss.yaml
// Loads newest version file as FeedDefinitionList if version files exist
type FileFeedDefinitionProvider struct {
	baseDir    string
	versionDir string
}

func NewFileFeedDefinitionProvider(dir string) (FeedDefinitionProvider, error) {
	versionDir := filepath.Join(dir, "version")

	// Create version directory if it doesn't exist
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create version directory: %w", err)
		}
	}

	return &FileFeedDefinitionProvider{
		baseDir:    dir,
		versionDir: versionDir,
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

func (p *FileFeedDefinitionProvider) getLatestVersionFile() (string, error) {
	files, err := os.ReadDir(p.versionDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to read version directory: %w", err)
	}

	if len(files) == 0 {
		return "", nil
	}

	prefix := FILE_NAME[:len(FILE_NAME)-5] + "_v"
	var versionFiles []string

	for _, file := range files {
		name := file.Name()
		if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".yaml") {
			versionFiles = append(versionFiles, name)
		}
	}

	if len(versionFiles) == 0 {
		return "", nil
	}

	// バージョンファイルを時間順にソート
	sort.Slice(versionFiles, func(i, j int) bool {
		// バージョン番号を比較
		vi := strings.Split(versionFiles[i], "_")[1]
		vj := strings.Split(versionFiles[j], "_")[1]
		viNum, _ := strconv.Atoi(strings.TrimPrefix(vi, "v"))
		vjNum, _ := strconv.Atoi(strings.TrimPrefix(vj, "v"))

		if viNum != vjNum {
			return viNum > vjNum
		}

		// 同じバージョン番号の場合はタイムスタンプで比較
		ti := strings.Split(versionFiles[i], "_")[2]
		tj := strings.Split(versionFiles[j], "_")[2]
		return ti > tj
	})

	return filepath.Join(p.versionDir, versionFiles[0]), nil
}

func (p *FileFeedDefinitionProvider) GetFeedDefinitionList() (*FeedDefinitionList, error) {
	// パスの検証
	if _, err := os.Stat(p.baseDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("directory not found: %s", p.baseDir)
	}

	// 最新のバージョンファイルを取得
	latestVersionFile, err := p.getLatestVersionFile()
	if err != nil {
		return nil, fmt.Errorf("failed to get latest version file: %w", err)
	}

	var data []byte
	if latestVersionFile != "" {
		// 最新のバージョンファイルが存在する場合はそれを読み込む
		data, err = os.ReadFile(latestVersionFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read version file: %w", err)
		}
	} else {
		// バージョンファイルが存在しない場合はオリジナルファイルを確認
		feedListPath := filepath.Join(p.baseDir, FILE_NAME)
		if _, err := os.Stat(feedListPath); os.IsNotExist(err) {
			// ファイルが存在しない場合は空のリストを返す
			return &FeedDefinitionList{Feeds: []FeedDefinition{}}, nil
		}

		data, err = os.ReadFile(feedListPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read feed list file: %w", err)
		}

		// 初回読み込み時にバージョンファイルとして保存
		if err := p.saveVersionFile(data); err != nil {
			return nil, fmt.Errorf("failed to save initial version file: %w", err)
		}
	}

	var list FeedDefinitionList
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("failed to parse feed list yaml: %w", err)
	}

	return &list, nil
}

func (p *FileFeedDefinitionProvider) getNextVersionNumber() (int, error) {
	// バージョンディレクトリ内のファイルを取得
	files, err := os.ReadDir(p.versionDir)
	if err != nil {
		return 1, fmt.Errorf("failed to read version directory: %w", err)
	}

	if len(files) == 0 {
		return 1, nil
	}

	// ファイル名からバージョン番号を抽出
	prefix := FILE_NAME[:len(FILE_NAME)-5] + "_v"
	maxVersion := 0

	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		// "feedlist_v1_20230101_120000.yaml" からバージョン番号を抽出
		parts := strings.Split(name, "_")
		if len(parts) < 2 {
			continue
		}

		versionStr := strings.TrimPrefix(parts[1], "v")
		version, err := strconv.Atoi(versionStr)
		if err != nil {
			continue
		}

		if version > maxVersion {
			maxVersion = version
		}
	}

	return maxVersion + 1, nil
}

func (p *FileFeedDefinitionProvider) saveVersionFile(data []byte) error {
	nextVersion, err := p.getNextVersionNumber()
	if err != nil {
		return fmt.Errorf("failed to get next version number: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	versionFileName := fmt.Sprintf("%s_v%d_%s.yaml", FILE_NAME[:len(FILE_NAME)-5], nextVersion, timestamp)
	versionPath := filepath.Join(p.versionDir, versionFileName)

	return os.WriteFile(versionPath, data, 0644)
}

func (p *FileFeedDefinitionProvider) AddFeedDefinition(def FeedDefinition) error {
	list, err := p.GetFeedDefinitionList()
	if err != nil {
		return fmt.Errorf("failed to get feed list: %w", err)
	}

	// 既存のフィードをチェック
	for _, d := range list.Feeds {
		if d.ID == def.ID {
			return fmt.Errorf("feed already exists: %s", def.ID)
		}
	}

	// フィードを追加
	list.Feeds = append(list.Feeds, def)

	// YAMLに変換
	data, err := yaml.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal feed list: %w", err)
	}

	// バージョンファイルに保存
	if err := p.saveVersionFile(data); err != nil {
		return fmt.Errorf("failed to save version file: %w", err)
	}

	return nil
}

func (p *FileFeedDefinitionProvider) DeleteFeedDefinition(feedId string) error {
	list, err := p.GetFeedDefinitionList()
	if err != nil {
		return fmt.Errorf("failed to get feed list: %w", err)
	}

	// フィードを検索して削除
	found := false
	newFeeds := make([]FeedDefinition, 0, len(list.Feeds))
	for _, d := range list.Feeds {
		if d.ID == feedId {
			found = true
			continue
		}
		newFeeds = append(newFeeds, d)
	}

	if !found {
		return fmt.Errorf("feed not found: %s", feedId)
	}

	list.Feeds = newFeeds

	// YAMLに変換
	data, err := yaml.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal feed list: %w", err)
	}

	// バージョンファイルに保存
	if err := p.saveVersionFile(data); err != nil {
		return fmt.Errorf("failed to save version file: %w", err)
	}

	return nil
}

func (p *FileFeedDefinitionProvider) UpdateFeedDefinition(newDef FeedDefinition) error {
	feedId := newDef.ID
	list, err := p.GetFeedDefinitionList()
	if err != nil {
		return fmt.Errorf("failed to get feed list: %w", err)
	}

	// フィードを検索して更新
	found := false
	for i, d := range list.Feeds {
		if d.ID == feedId {
			list.Feeds[i] = newDef
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("feed not found: %s", feedId)
	}

	// YAMLに変換
	data, err := yaml.Marshal(list)
	if err != nil {
		return fmt.Errorf("failed to marshal feed list: %w", err)
	}

	// バージョンファイルに保存
	if err := p.saveVersionFile(data); err != nil {
		return fmt.Errorf("failed to save version file: %w", err)
	}

	return nil
}
