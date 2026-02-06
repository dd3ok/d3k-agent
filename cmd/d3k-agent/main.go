package main

import (
	"context"
	"fmt"
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
	godotenv.Load()
	fmt.Println("🤖 D3K Integrated Agent Starting... [v1.0.5]")

	ctx := context.Background()
	store, _ := storage.NewJSONStorage("data/storage.json")

	myBrain, _ := brain.NewGeminiBrain(ctx, os.Getenv("GEMINI_API_KEY"))
	if myBrain != nil { fmt.Println("🧠 Brain connected") }

	var ui ports.Interaction
	tgToken, tgChatID := os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID")
	if tgToken != "" { ui, _ = telegram.NewTelegramUI(tgToken, tgChatID); fmt.Println("📲 Telegram connected") }

	agents := []ports.Site{botmadang.NewClient(store)}
	for _, agent := range agents { agent.Initialize(ctx) }

	for {
		fmt.Printf("\n--- 🔄 Check Cycle (%s) ---\n", time.Now().Format("15:04:05"))
		for _, agent := range agents {
			processAgent(ctx, agent, myBrain, ui, store)
		}
		time.Sleep(10 * time.Minute)
	}
}

func processAgent(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	fmt.Printf("Checking %s... ", agent.Name())
	handleNotifications(ctx, agent, brain, ui, store)
	handleProactiveCommenting(ctx, agent, brain, ui, store)
	handleDailyPosting(ctx, agent, brain, ui, store)
}

func handleNotifications(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, _, _ := store.GetCommentStats(agent.Name())
	if count >= 20 { return }

	notifs, _ := agent.GetNotifications(ctx, true)
	if len(notifs) == 0 { fmt.Print("0 notifs. "); return }

	groups := make(map[string]struct{ title, latestCID string; contents, notifIDs []string })
	for _, n := range notifs {
		if n.Type != "comment_on_post" && n.Type != "reply_to_comment" { continue }
		g := groups[n.PostID]; g.title = n.PostTitle; g.latestCID = n.CommentID
		g.contents = append(g.contents, fmt.Sprintf("- %s: %s", n.ActorName, n.Content))
		g.notifIDs = append(g.notifIDs, n.ID)
		groups[n.PostID] = g
	}

	for pid, g := range groups {
		if brain == nil || ui == nil { continue }
		if count >= 20 { break }
		reply, _ := brain.GenerateReply(ctx, g.title, strings.Join(g.contents, "\n"))
		
		tgTitle := fmt.Sprintf("💬 통합 답글 승인 요청 (%d개)", len(g.notifIDs))
		tgBody := fmt.Sprintf("📍 *게시글*: %s\n\n🤖 *답글*:\n%s", g.title, reply)
		
		action, _ := ui.Confirm(ctx, tgTitle, tgBody)
		if action == ports.ActionApprove {
			if err := agent.ReplyToComment(ctx, pid, g.latestCID, reply); err == nil {
				for _, nid := range g.notifIDs { agent.MarkNotificationRead(ctx, nid) }
				store.IncrementCommentCount(agent.Name(), today)
				count++
			}
		}
	}
}

func handleProactiveCommenting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, _, _ := store.GetCommentStats(agent.Name())
	if count >= 20 || brain == nil || ui == nil { return }

	posts, _ := agent.GetRecentPosts(ctx, 5)
	for _, p := range posts {
		if done, _ := store.IsProactiveDone(agent.Name(), p.ID); done { continue }
		if count >= 20 { break }
		
		score, reason, _ := brain.EvaluatePost(ctx, p)
		if score >= 7 {
			reply, _ := brain.GenerateReply(ctx, p.Title, p.Content)
			action, _ := ui.Confirm(ctx, fmt.Sprintf("🌟 선제적 댓글 승인 (점수:%d)", score), fmt.Sprintf("📍 제목: %s\n📝 이유: %s\n\n🤖 댓글:\n%s", p.Title, reason, reply))
			if action == ports.ActionApprove {
				if err := agent.CreateComment(ctx, p.ID, reply); err == nil {
					store.MarkProactive(agent.Name(), p.ID)
					store.IncrementCommentCount(agent.Name(), today)
					count++
					break // 한 사이클에 하나만
				}
			} else {
				store.MarkProactive(agent.Name(), p.ID)
			}
		}
	}
}

func handleDailyPosting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, lastDate, lastTs, _ := store.GetPostStats(agent.Name())
	if lastDate != today { count = 0 }

	if count < 4 && (lastTs == 0 || time.Since(time.Unix(lastTs, 0)) >= 2*time.Hour) && rand.Float32() < 0.4 {
		topics := []string{"금융 및 경제 트렌드", "최신 기술 동향", "일상의 지혜", "자기계발"}
		postJSON, _ := brain.GeneratePost(ctx, topics[rand.Intn(len(topics))])
		
		action, _ := ui.Confirm(ctx, "🚀 새 게시글 승인 요청", postJSON)
		if action == ports.ActionApprove {
			if err := agent.CreatePost(ctx, domain.Post{Content: postJSON, Source: agent.Name()}); err == nil {
				store.IncrementPostCount(agent.Name(), today, time.Now().Unix())
			}
		}
	}
}