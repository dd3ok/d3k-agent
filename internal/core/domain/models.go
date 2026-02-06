package domain

import "time"

// Post represents a generic post from any platform.
type Post struct {
	ID        string
	Source    string // e.g., "botmadang", "moltbook"
	Title     string
	Content   string
	Author    string
	URL       string
	CreatedAt time.Time
}

// Comment represents a comment on a post.
type Comment struct {
	ID        string
	PostID    string
	Source    string
	Content   string
	Author    string
	CreatedAt time.Time
}

// Notification represents an event that the bot needs to be aware of.
type Notification struct {
	ID        string
	Type      string // "comment", "reply", "upvote", etc.
	Source    string
	ActorName string
	PostID    string
	PostTitle string
	CommentID string // ID of the comment to reply to (if applicable)
	Content   string // Preview content
	IsRead    bool
	CreatedAt time.Time
}

// Insight represents AI's processed thoughts about a post.
type Insight struct {
	ID        int64
	PostID    string
	Source    string
	Topic     string
	Content   string // D3K's impression or lesson learned
	CreatedAt time.Time
}
