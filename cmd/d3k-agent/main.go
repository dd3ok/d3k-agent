package main

import (
	"bufio"
	"context"
	"encoding/json"
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
	fmt.Println("🤖 d3k Integrated Agent Starting... [v1.2.1-UI-Fix]")

	ctx := context.Background()
	var store ports.Storage
	var err error

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		store, err = storage.NewPostgresStorage(ctx, dbURL)
		if err == nil { fmt.Println("🐘 Storage: PostgreSQL Connected") }
	}
	if store == nil {
		store, _ = storage.NewJSONStorage("data/storage.json")
		fmt.Println("📄 Storage: JSON File Mode")
	}

	myBrain, _ := brain.NewGeminiBrain(ctx, os.Getenv("GEMINI_API_KEY"))
	ui, _ := telegram.NewTelegramUI(os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID"))

	agents := []ports.Site{
		botmadang.NewClient(store),
	}
	for _, agent := range agents { agent.Initialize(ctx) }

	trigger := make(chan bool, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			reader.ReadString('\n')
			trigger <- true
		}
	}()

	fmt.Println("🚀 System fully operational (UI Fixed).")

	firstRun := true
	for {
		fmt.Printf("\n--- 🔄 Check Cycle (%s) ---\n", time.Now().Format("15:04:05"))
		for _, agent := range agents {
			processAgent(ctx, agent, myBrain, ui, store, firstRun)
		}
		firstRun = false

		fmt.Println("\nWaiting 10 minutes...")
		select {
		case <-time.After(10 * time.Minute):
		case <-trigger:
			fmt.Println("⚡ Manual trigger!")
		}
	}
}

func processAgent(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage, firstRun bool) {
	fmt.Printf("Checking %s... ", agent.Name())
	handleNotifications(ctx, agent, brain, ui, store)
	handleProactiveCommenting(ctx, agent, brain, ui, store)
	handleDailyPosting(ctx, agent, brain, ui, store, firstRun)
	learnFromCommunity(ctx, agent, brain, store)
}

func learnFromCommunity(ctx context.Context, agent ports.Site, brain ports.Brain, store ports.Storage) {
	posts, err := agent.GetRecentPosts(ctx, 3)
	if err != nil || brain == nil { return }
	for _, p := range posts {
		insightText, err := brain.SummarizeInsight(ctx, p)
		if err == nil && insightText != "" {
			store.SaveInsight(ctx, domain.Insight{PostID: p.ID, Source: agent.Name(), Topic: p.Title, Content: insightText})
		}
	}
}

func handleNotifications(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, _, _ := store.GetCommentStats(agent.Name())
	if count >= 20 { return }

	notifs, _ := agent.GetNotifications(ctx, true)
	if len(notifs) == 0 { fmt.Print("0 notifs. "); return }

	groups := make(map[string]struct{ title, latestCID, postID string; contents, notifIDs []string })
	for _, n := range notifs {
		if n.Type != "comment_on_post" && n.Type != "reply_to_comment" { continue }
		g := groups[n.PostID]; g.title = n.PostTitle; g.latestCID = n.CommentID; g.postID = n.PostID
		g.contents = append(g.contents, fmt.Sprintf("- %s: %s", n.ActorName, n.Content))
		g.notifIDs = append(g.notifIDs, n.ID)
		groups[n.PostID] = g
	}

	for pid, g := range groups {
		if brain == nil || ui == nil || count >= 20 { break }
		actionID := "reply_" + pid
		if pending, _ := store.IsPending(actionID); pending { continue }

		reply, _ := brain.GenerateReply(ctx, g.title, strings.Join(g.contents, "\n"))
		
		go func(pid, latestCID, title, reply string, notifIDs []string, contents []string) {
			store.SetPending(actionID)
			defer store.ClearPending(actionID)

			tgTitle := fmt.Sprintf("💬 [%s] 답글 승인", agent.Name())
			link := fmt.Sprintf("🔗 [원문 보기](https://botmadang.org/post/%s)", pid)
			
			// 원문 댓글 요약과 내 답글 구성
			tgBody := fmt.Sprintf("📍 게시글: %s\n%s\n\n💬 상대방:\n%s\n\n🤖 d3k 답글:\n%s", 
				title, link, strings.Join(contents, "\n"), reply)
			
			action, err := ui.Confirm(ctx, tgTitle, tgBody)
			if err == nil && action == ports.ActionApprove {
				if err := agent.ReplyToComment(ctx, pid, latestCID, reply); err == nil {
					for _, nid := range notifIDs { agent.MarkNotificationRead(ctx, nid) }
					store.IncrementCommentCount(agent.Name(), today)
				}
			}
		}(pid, g.latestCID, g.title, reply, g.notifIDs, g.contents)
	}
}

func handleProactiveCommenting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, _, _ := store.GetCommentStats(agent.Name())
	if count >= 20 || brain == nil || ui == nil { return }

	posts, _ := agent.GetRecentPosts(ctx, 5)
	for _, p := range posts {
		if done, _ := store.IsProactiveDone(agent.Name(), p.ID); done || count >= 20 { continue }
		actionID := "proactive_" + p.ID
		if pending, _ := store.IsPending(actionID); pending { continue }

		score, reason, _ := brain.EvaluatePost(ctx, p)
		if score >= 7 {
			reply, _ := brain.GenerateReply(ctx, p.Title, p.Content)
			
			go func(p domain.Post, score int, reason, reply string) {
				store.SetPending(actionID)
				defer store.ClearPending(actionID)

				tgTitle := fmt.Sprintf("🌟 [%s] 선제 댓글 (%d점)", agent.Name(), score)
				link := fmt.Sprintf("🔗 [원문 보기](%s)", p.URL)
				
				// 원문 일부와 내 댓글 구성
				preview := p.Content
				if len(preview) > 150 { preview = preview[:150] + "..." }
				
				tgBody := fmt.Sprintf("📍 제목: %s\n%s\n\n📄 원문 요약:\n%s\n\n🤖 d3k 댓글:\n%s\n\n💡 이유: %s", 
					p.Title, link, preview, reply, reason)
				
				action, err := ui.Confirm(ctx, tgTitle, tgBody)
				if err == nil && action == ports.ActionApprove {
					if err := agent.CreateComment(ctx, p.ID, reply); err == nil {
						store.MarkProactive(agent.Name(), p.ID)
						store.IncrementCommentCount(agent.Name(), today)
					}
				} else if action == ports.ActionSkip {
					store.MarkProactive(agent.Name(), p.ID)
				}
			}(p, score, reason, reply)
		}
	}
}

func handleDailyPosting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage, firstRun bool) {
	today := time.Now().Format("2006-01-02")
	count, lastDate, lastTs, _ := store.GetPostStats(agent.Name())
	if lastDate != today { count = 0 }

	canPost := firstRun || (lastTs == 0 || time.Since(time.Unix(lastTs, 0)) >= 2*time.Hour)
	if count < 4 && canPost {
		if !firstRun && rand.Float32() > 0.4 { return }
		
		topics := []string{"금융 경제", "IT 기술", "일상 지혜", "커리어"}
		topic := topics[rand.Intn(len(topics))]
		actionID := "post_" + today + "_" + topic
		if pending, _ := store.IsPending(actionID); pending { return }

		postJSON, _ := brain.GeneratePost(ctx, topic)
		
		go func(topic, rawJSON string) {
			store.SetPending(actionID)
			defer store.ClearPending(actionID)

			var p struct { Title, Content string }
			json.Unmarshal([]byte(rawJSON), &p)

			tgTitle := fmt.Sprintf("🚀 [%s] 새 글 승인 (%s)", agent.Name(), topic)
			tgBody := fmt.Sprintf("📌 제목: %s\n\n📝 내용:\n%s", p.Title, p.Content)
			
			action, err := ui.Confirm(ctx, tgTitle, tgBody)
			if err == nil && action == ports.ActionApprove {
				if err := agent.CreatePost(ctx, domain.Post{Content: rawJSON, Source: agent.Name()}); err == nil {
					store.IncrementPostCount(agent.Name(), today, time.Now().Unix())
				}
			}
		}(topic, postJSON)
	}
}
