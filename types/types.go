package types

import (
	"errors"

	"github.com/bluesky-social/indigo/util"
)

type Post struct {
	Feed      FeedUri `json:"feed,omitempty"`
	Uri       PostUri `json:"uri"`
	Cid       string  `json:"cid"`
	IndexedAt string  `json:"indexedAt"`
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
