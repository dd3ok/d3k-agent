package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"d3k-agent/internal/brain"
	"d3k-agent/internal/core/domain"
	"d3k-agent/internal/core/ports"
	"d3k-agent/internal/sites/botmadang"
	"d3k-agent/internal/storage"
	"d3k-agent/internal/ui/telegram"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		fmt.Println("ℹ️  Note: .env file not found, using system environment variables.")
	}

	fmt.Println("🤖 D3K Integrated Agent Starting... [v1.0.1]")

	ctx := context.Background()

	store, err := storage.NewJSONStorage("data/storage.json")
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	brainKey := os.Getenv("GEMINI_API_KEY")
	var myBrain ports.Brain
	if brainKey != "" {
		myBrain, err = brain.NewGeminiBrain(ctx, brainKey)
		if err == nil {
			fmt.Println("🧠 Brain connected (Gemini)")
		}
	}

	var ui ports.Interaction
	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	tgChatID := os.Getenv("TELEGRAM_CHAT_ID")
	if tgToken != "" && tgChatID != "" {
		ui, err = telegram.NewTelegramUI(tgToken, tgChatID)
		if err == nil {
			fmt.Println("📲 Telegram Bot connected")
		}
	}

	agents := []ports.Site{
		botmadang.NewClient(store),
	}
	for _, agent := range agents {
		if err := agent.Initialize(ctx); err != nil {
			log.Fatalf("❌ Critical Error: Failed to initialize %s: %v", agent.Name(), err)
		}
	}

	// Main loop
	for {
		fmt.Printf("\n--- 🔄 Check Cycle (%s) ---\n", time.Now().Format("15:04:05"))
		for _, agent := range agents {
			processAgent(ctx, agent, myBrain, ui, store)
		}

		fmt.Println("\nWaiting 10 minutes for next cycle...")
		select {
		case <-time.After(10 * time.Minute):
		}
	}
}

func processAgent(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	fmt.Printf("Checking %s... ", agent.Name())

	handleNotifications(ctx, agent, brain, ui, store)
	handleDailyPosting(ctx, agent, brain, ui, store)
	
	posts, _ := agent.GetRecentPosts(ctx, 3)
	if len(posts) > 0 { fmt.Print("Posts OK. ") }
}

type notificationGroup struct {
	PostID    string
	PostTitle string
	LatestCID string
	Contents  []string
	NotifIDs  []string
}

func handleNotifications(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, lastDate, _ := store.GetCommentStats(agent.Name())
	if lastDate != today { count = 0 }
	
	if count >= 12 {
		fmt.Print("Comment quota reached (12/12). ")
		return
	}

	notifs, err := agent.GetNotifications(ctx, true)
	if err != nil || len(notifs) == 0 { return }

	groups := make(map[string]*notificationGroup)
	for _, n := range notifs {
		if n.Type != "comment_on_post" && n.Type != "reply_to_comment" { continue }
		
		if _, ok := groups[n.PostID]; !ok {
			groups[n.PostID] = &notificationGroup{
				PostID:    n.PostID,
				PostTitle: n.PostTitle,
				LatestCID: n.CommentID,
			}
		}
		groups[n.PostID].Contents = append(groups[n.PostID].Contents, fmt.Sprintf("- %s: %s", n.ActorName, n.Content))
		groups[n.PostID].NotifIDs = append(groups[n.PostID].NotifIDs, n.ID)
	}

	if len(groups) > 0 {
		fmt.Printf("\n🔔 Found notifications across %d posts.\n", len(groups))
	}

	for _, g := range groups {
		if brain == nil || ui == nil { continue }
		if count >= 12 { break }

		combinedComments := strings.Join(g.Contents, "\n")
		
		for {
			replyContent, err := brain.GenerateReply(ctx, g.PostTitle, combinedComments)
			if err != nil { break }

			fmt.Printf("    🤖 Generated consolidated reply for post: %s\n", g.PostTitle)
			action, _ := ui.Confirm(ctx, fmt.Sprintf("통합 답글 승인 요청 (%d개 댓글)", len(g.NotifIDs)), replyContent)

			if action == ports.ActionApprove {
				if err := agent.ReplyToComment(ctx, g.PostID, g.LatestCID, replyContent); err == nil {
					fmt.Println("    ✅ Consolidated reply sent!")
					for _, nid := range g.NotifIDs {
						agent.MarkNotificationRead(ctx, nid)
					}
					store.IncrementCommentCount(agent.Name(), today)
					count++
				}
				break
			} else if action == ports.ActionRegenerate {
				continue
			} else {
				break
			}
		}
	}
}

func handleDailyPosting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	if brain == nil || ui == nil { return }

	today := time.Now().Format("2006-01-02")
	count, lastDate, lastTs, _ := store.GetPostStats(agent.Name())
	if lastDate != today { count = 0 }

	if count < 4 {
		now := time.Now()
		elapsed := now.Sub(time.Unix(lastTs, 0))
		if elapsed < 2*time.Hour && lastTs > 0 { return }

		// Chance per cycle (10 mins)
		if rand.Float32() > 0.4 { return }

		topics := []string{"금융 및 경제 트렌드", "최신 기술 동향", "일상의 지혜와 인사이트", "자기계발 및 커리어"}
		attempt := rand.Intn(len(topics))

		for {
			currentTopic := topics[attempt % len(topics)]
			postJSON, err := brain.GeneratePost(ctx, currentTopic)
			if err != nil { break }

			action, _ := ui.Confirm(ctx, fmt.Sprintf("새 게시글 승인 요청 (%s)", currentTopic), postJSON)

			if action == ports.ActionApprove {
				if err := agent.CreatePost(ctx, domain.Post{Content: postJSON, Source: agent.Name()}); err == nil {
					fmt.Println("    ✅ Post published!")
					store.IncrementPostCount(agent.Name(), today, now.Unix())
				}
				break
			} else if action == ports.ActionRegenerate {
				attempt++
				continue
			} else {
				break
			}
		}
	}
}
