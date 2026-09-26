package types

import (
	"errors"

	"github.com/bluesky-social/indigo/util"
)

var ErrInvalidPostReason = errors.New("post reason must specify exactly one of repost or pin")

type PostReason struct {
	Repost *string `json:"repost,omitempty"`
	Pin    bool    `json:"pin,omitempty"`
}

type PostMetadata struct {
	FeedContext *string
	Reason      *PostReason
}

func (r *PostReason) Validate() error {
	if r == nil {
		return nil
	}
	if (r.Repost != nil) == r.Pin {
		return ErrInvalidPostReason
	}
	return nil
}

type Post struct {
	Feed        FeedUri     `json:"feed,omitempty"`
	Uri         PostUri     `json:"uri"`
	Cid         string      `json:"cid"`
	IndexedAt   string      `json:"indexedAt"`
	Langs       []string    `json:"langs,omitempty"`
	FeedContext *string     `json:"feedContext,omitempty"`
	Reason      *PostReason `json:"reason,omitempty"`
}

type FeedUri string

func (f FeedUri) Validate() error {
	// if f is empty, return false
	p, err := util.ParseAtUri(string(f))
	if err != nil {
		return err
	}
	//at://did:plc:userdid/app.bsky.feed.generator/samplefeed
	if p.Collection != "app.bsky.feed.generator" {
		return errors.New("invalid collection at feed")
	}
	return nil
}

type PostUri string

func (u PostUri) Validate() error {
	p, err := util.ParseAtUri(string(u))
	if err != nil {
		return err
	}
	//at://did:plc:did:plc:userdid/app.bsky.feed.generator/samplefeed
	if p.Collection != "app.bsky.feed.post" {
		return errors.New("invalid collection at uri")
	}
	return nil
}
