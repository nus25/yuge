package subscriber

//temporary removed until feed package refactoring is done

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/gin-gonic/gin"
	"github.com/nus25/yuge/feed/metrics"
	"github.com/nus25/yuge/types"
)

// APIハンドラー
type FeedApiHandler struct {
	feedService *FeedService
}

// NewAPIHandler はフィードを操作するAPIハンドラーを作成します
func NewFeedApiHandler(fs *FeedService) *FeedApiHandler {
	return &FeedApiHandler{
		feedService: fs,
	}
}

// エラーレスポンスを標準化するヘルパー関数
func respondWithError(c *gin.Context, statusCode int, message string, err error) {
	response := gin.H{
		"error": message,
	}
	if err != nil {
		response["details"] = err.Error()
	}
	c.JSON(statusCode, response)
}

func (h *FeedApiHandler) ValidateFeedId() gin.HandlerFunc {
	return func(c *gin.Context) {
		feedId := c.Param("feedid")
		if _, exists := h.feedService.GetFeedInfo(feedId); !exists {
			c.JSON(http.StatusNotFound, gin.H{
				"error":  "feed not found",
				"feedid": feedId,
			})
			c.Abort()
			return
		}
		c.Next()
	}
}

// //////////////////
// // feed APIs
type ListFeedResponse struct {
	ID         string         `json:"id"`
	Definition FeedDefinition `json:"definition"`
	Status     *FeedStatus    `json:"status"`
}

type FeedStatusResponse struct {
	LastStatus  string `json:"last_status"`
	LastUpdated string `json:"last_updated"`
	Error       string `json:"error,omitempty"`
}

func (h *FeedApiHandler) ListFeed(c *gin.Context) {
	feeds := h.feedService.GetAllFeeds()
	response := make([]ListFeedResponse, 0, len(feeds))

	for id, fi := range feeds {
		if fi.Feed != nil {
			response = append(response, ListFeedResponse{
				ID:         id,
				Definition: fi.Definition,
				Status:     &fi.Status,
			})
		} else {
			response = append(response, ListFeedResponse{
				ID:         id,
				Definition: fi.Definition,
				Status:     &fi.Status,
			})
		}
	}

	c.JSON(200, response)
}

// RegisterFeed - PUT /api/feed/:feedid に変更し、冪等性を持たせる
func (h *FeedApiHandler) RegisterFeed(c *gin.Context) {
	feedId := c.Param("feedid")

	var req struct {
		FeedURI       string `json:"uri"`
		ConfigFile    string `json:"configFile"`
		InactiveStart bool   `json:"inactiveStart"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	status := FeedStatusActive
	if req.InactiveStart {
		status = FeedStatusInactive
	}

	def := FeedDefinition{
		ID:            feedId,
		URI:           req.FeedURI,
		ConfigFile:    req.ConfigFile,
		InactiveStart: "false",
	}
	if req.InactiveStart {
		def.InactiveStart = "true"
	}

	// 既存のフィードがあるか確認
	_, exists := h.feedService.GetFeedInfo(feedId)

	var err error
	if exists {
		// 既存のフィードを更新
		err = h.feedService.ReloadFeed(context.Background(), feedId)
		if err == nil {
			c.JSON(http.StatusOK, gin.H{
				"message": "Feed updated successfully",
				"feedId":  feedId,
				"status":  status.String(),
			})
			return
		}
	} else {
		// 新規フィード作成
		err = h.feedService.CreateFeed(context.Background(), def, status)
		if err == nil {
			if h.feedService.definitionProvider != nil {
				h.feedService.definitionProvider.AddFeedDefinition(def)
			}
			c.JSON(http.StatusCreated, gin.H{
				"message": "Feed created successfully",
				"feedId":  feedId,
				"status":  status.String(),
			})
			return
		}
	}

	// エラー処理
	c.JSON(http.StatusInternalServerError, gin.H{
		"error":   "Failed to process feed",
		"details": err.Error(),
	})
}

func (h *FeedApiHandler) UnregisterFeed(c *gin.Context) {
	feedId := c.Param("feedid")
	// Check if feed exists
	_, exists := h.feedService.GetFeedInfo(feedId)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Feed not found",
		})
		return
	}

	// Delete the feed
	if err := h.feedService.DeleteFeed(feedId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Feed successfully deleted",
		"feedId":  feedId,
	})
}

type FeedInfoResponse struct {
	ID      string           `json:"id"`
	URI     string           `json:"uri"`
	Status  *FeedStatus      `json:"status"`
	Config  interface{}      `json:"config"`
	Metrics *metrics.Metrics `json:"metrics"`
}

func (h *FeedApiHandler) GetFeedInfo(c *gin.Context) {
	feedId := c.Param("feedid")
	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusNotFound, gin.H{
			"error": fmt.Sprintf("feed %s is in error state: %s", feedId, fi.Status.Error),
		})
		return
	}

	response := FeedInfoResponse{
		ID:     feedId,
		Status: &fi.Status,
	}

	if fi.Feed != nil {
		response.URI = fi.Feed.FeedUri()
		response.Metrics = fi.Feed.Metrics()
		response.Config = fi.Feed.Config()
	}

	c.JSON(200, response)
}

type UpdateStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active inactive error"`
}

type StatusResponse struct {
	Status *FeedStatus `json:"status"`
}

func (h *FeedApiHandler) GetFeedStatus(c *gin.Context) {
	feedId := c.Param("feedid")
	fi, _ := h.feedService.GetFeedInfo(feedId)

	c.JSON(http.StatusOK, StatusResponse{
		Status: &fi.Status,
	})
}

func (h *FeedApiHandler) UpdateFeedStatus(c *gin.Context) {
	feedId := c.Param("feedid")

	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError || fi.Feed == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot update status: feed is in error state or not initialized",
		})
		return
	}

	var req UpdateStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	var status Status
	switch req.Status {
	case "active":
		status = FeedStatusActive
	case "inactive":
		status = FeedStatusInactive
	case "error":
		status = FeedStatusError
	default:
		status = FeedStatusUnknown
	}
	if status != FeedStatusActive && status != FeedStatusInactive && status != FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid status: must be one of active, inactive, error",
		})
		return
	}

	if err := h.feedService.UpdateStatus(feedId, status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update status: " + err.Error(),
		})
		return
	}
	fi, _ = h.feedService.GetFeedInfo(feedId)
	c.JSON(http.StatusOK, StatusResponse{
		Status: &fi.Status,
	})
}

func (h *FeedApiHandler) ReloadFeed(c *gin.Context) {
	feedId := c.Param("feedid")

	err := h.feedService.ReloadFeed(context.Background(), feedId)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return

	}

	c.JSON(200, gin.H{
		"message": "reload feed completed.",
		"id":      feedId,
	})
}

func (h *FeedApiHandler) ClearFeed(c *gin.Context) {
	feedId := c.Param("feedid")
	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot clear feed: feed is in error state",
		})
		return
	}
	if err := fi.Feed.Clear(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{
		"message": "Clear feed completed.",
	})
}

////////////////////
//// feedconfig apis

func (h *FeedApiHandler) GetConfig(c *gin.Context) {
	feedId := c.Param("feedid")
	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot get config: feed is in error state",
		})
		return
	}
	config := fi.Feed.Config()
	c.JSON(200, config)
}

type GetAllPostsResponse struct {
	Posts []types.Post `json:"posts"`
}

func (h *FeedApiHandler) GetAllPosts(c *gin.Context) {
	feedId := c.Param("feedid")
	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot get posts: feed is in error state",
		})
		return
	}
	posts := fi.Feed.ListPost("")
	c.JSON(http.StatusOK, GetAllPostsResponse{
		Posts: posts,
	})
}

type GetPostsByDidResponse struct {
	Posts []types.Post `json:"posts"`
}

func (h *FeedApiHandler) GetPostsByDid(c *gin.Context) {
	feedId := c.Param("feedid")
	did := c.Param("did")

	if _, err := syntax.ParseDID(did); err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid DID format", err)
		return
	}

	fi, _ := h.feedService.GetFeedInfo(feedId)
	posts := fi.Feed.ListPost(did)
	c.JSON(http.StatusOK, GetPostsByDidResponse{
		Posts: posts,
	})
}

type GetPostByRkeyResponse struct {
	Post types.Post `json:"post"`
}

func (h *FeedApiHandler) GetPostByRkey(c *gin.Context) {
	feedId := c.Param("feedid")
	did := c.Param("did")
	rkey := c.Param("rkey")

	if _, err := syntax.ParseDID(did); err != nil {
		respondWithError(c, http.StatusBadRequest, "Invalid DID format", err)
		return
	}

	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot get post: feed is in error state",
		})
		return
	}
	post, exists := fi.Feed.GetPost(did, rkey)
	if !exists {
		respondWithError(c, http.StatusNotFound, "Post not found", nil)
		return
	}

	c.JSON(http.StatusOK, GetPostByRkeyResponse{
		Post: post,
	})
}

type AddPostResponse struct {
	Message string     `json:"message"`
	Post    types.Post `json:"post"`
}

func (h *FeedApiHandler) AddPost(c *gin.Context) {
	feedId := c.Param("feedid")
	did := c.Param("did")
	rkey := c.Param("rkey")

	// DIDの形式チェック
	if _, err := syntax.ParseDID(did); err != nil {
		c.JSON(400, gin.H{"error": "invalid did format"})
		return
	}

	// POSTデータを受け取る
	var req struct {
		CID       string   `json:"cid"`
		IndexedAt string   `json:"indexedAt"`
		Langs     []string `json:"langs,omitempty"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid request body"})
		return
	}

	// CIDの形式チェック
	if len(req.CID) == 0 {
		c.JSON(400, gin.H{"error": "invalid cid format: cid must not be empty"})
		return
	}

	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot add post: feed is in error state",
		})
		return
	}
	var t time.Time
	fmt.Println("ind:" + req.IndexedAt)
	if req.IndexedAt != "" {
		var err error
		t, err = time.Parse(time.RFC3339Nano, req.IndexedAt)
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid indexedAt format"})
			return
		}
	} else {
		t = time.Now()
	}

	if err := fi.Feed.AddPost(did, rkey, req.CID, t, req.Langs); err != nil {
		c.JSON(500, gin.H{"error": "failed to add post"})
		return
	}
	post := types.Post{
		Uri:       types.PostUri("at://" + did + "/app.bsky.feed.post/" + rkey),
		Cid:       req.CID,
		IndexedAt: t.UTC().Format(time.RFC3339Nano),
	}
	c.JSON(200, AddPostResponse{
		Message: "post added successfully",
		Post:    post,
	})
}

type DeletePostByDidResponse struct {
	Message string       `json:"message"`
	Deleted []types.Post `json:"deleted"`
}

func (h *FeedApiHandler) DeletePostByDid(c *gin.Context) {
	feedId := c.Param("feedid")
	did := c.Param("did")

	// DIDの形式チェック
	if _, err := syntax.ParseDID(did); err != nil {
		c.JSON(400, gin.H{"error": "invalid did format"})
		return
	}

	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot delete post: feed is in error state",
		})
		return
	}

	// 指定したdidのポストを全て削除する
	deleted, err := fi.Feed.DeletePostByDid(did)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to delete posts"})
		return
	}

	c.JSON(200, DeletePostByDidResponse{
		Message: "posts deleted successfully",
		Deleted: deleted,
	})
}

type DeletePostByRkeyResponse struct {
	Message string     `json:"message"`
	Deleted types.Post `json:"deleted"`
}

func (h *FeedApiHandler) DeletePost(c *gin.Context) {
	feedId := c.Param("feedid")
	// パスパラメータからDIDとRKeyを取得
	did := c.Param("did")
	rkey := c.Param("rkey")

	// DIDの形式チェック
	if _, err := syntax.ParseDID(did); err != nil {
		c.JSON(400, gin.H{"error": "invalid did format"})
		return
	}

	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot delete post: feed is in error state",
		})
		return
	}

	// RKeyの形式チェック
	if len(rkey) == 0 {
		c.JSON(400, gin.H{"error": "rkey must not be empty"})
		return
	}
	post, exists := fi.Feed.GetPost(did, rkey)
	if !exists {
		c.JSON(404, gin.H{"error": "post not found"})
		return
	}

	// ストアから削除
	fi.Feed.DeletePost(did, rkey)

	c.JSON(200, DeletePostByRkeyResponse{
		Message: "post deleted successfully",
		Deleted: post,
	})
}

type ProcessLogicBlockCommandRequest struct {
	Args map[string]string `json:"args,omitempty"`
}

func (h *FeedApiHandler) ProcessLogicBlockCommand(c *gin.Context) {
	feedId := c.Param("feedid")
	logicBlockName := c.Param("logicblockname")
	command := c.Param("command")
	var req ProcessLogicBlockCommandRequest
	var args map[string]string

	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid request format: " + err.Error(),
			})
			return
		}
		args = req.Args
	}

	fi, _ := h.feedService.GetFeedInfo(feedId)
	if fi.Status.LastStatus == FeedStatusError {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "cannot process command: feed is in error state",
		})
		return
	}
	msg, err := fi.Feed.ProcessCommand(logicBlockName, command, args)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"message": msg})
}
