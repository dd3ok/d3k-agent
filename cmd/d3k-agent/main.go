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
	fmt.Println("🤖 d3k Integrated Agent Starting... [v1.3.5-Sync-Return]")

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

	fmt.Println("🚀 System fully operational (Synchronous Mode).")

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

	notifs, err := agent.GetNotifications(ctx, true)
	if err != nil || len(notifs) == 0 { return }

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

		peerText := strings.Join(g.contents, "\n")
		reply, err := brain.GenerateReply(ctx, g.title, peerText)
		if err != nil { continue }

		summary, _ := brain.SummarizeInsight(ctx, domain.Post{Content: peerText})
		if summary == "" { summary = "동료들의 새로운 의견이 도착했습니다." }

		tgTitle := fmt.Sprintf("💬 [%s] 답글 승인", agent.Name())
		link := fmt.Sprintf("🔗 [원문](https://botmadang.org/post/%s)", pid)
		tgBody := fmt.Sprintf("📍 글: %s\n%s\n\n📄 요약: %s\n\n🤖 답글: %s", 
			g.title, link, summary, reply)
		
		action, err := ui.Confirm(ctx, tgTitle, tgBody)
		if err == nil && action == ports.ActionApprove {
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
		if done, _ := store.IsProactiveDone(agent.Name(), p.ID); done || count >= 20 { continue }

		score, _, err := brain.EvaluatePost(ctx, p)
		if err != nil || score < 7 { continue }

		reply, err := brain.GenerateReply(ctx, p.Title, p.Content)
		if err != nil { continue }

		summary, _ := brain.SummarizeInsight(ctx, p)
		if summary == "" { summary = p.Title }

		tgTitle := fmt.Sprintf("🌟 [%s] 선제 댓글 (%d점)", agent.Name(), score)
		link := fmt.Sprintf("🔗 [원문](%s)", p.URL)
		tgBody := fmt.Sprintf("📍 제목: %s\n%s\n\n📄 요약: %s\n\n🤖 댓글: %s", 
			p.Title, link, summary, reply)
		
		action, err := ui.Confirm(ctx, tgTitle, tgBody)
		if err == nil && action == ports.ActionApprove {
			if err := agent.CreateComment(ctx, p.ID, reply); err == nil {
				store.MarkProactive(agent.Name(), p.ID)
				store.IncrementCommentCount(agent.Name(), today)
				count++
			}
		} else if action == ports.ActionSkip {
			store.MarkProactive(agent.Name(), p.ID)
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

		raw, err := brain.GeneratePost(ctx, topic)
		if err != nil { return }

		cleaned := raw
		if start := strings.Index(raw, "{"); start != -1 {
			if end := strings.LastIndex(raw, "}"); end != -1 && end > start {
				cleaned = raw[start : end+1]
			}
		}

		var p struct {
			Title     string `json:"title"`
			Content   string `json:"content"`
			Submadang string `json:"submadang"`
		}
		
		err = json.Unmarshal([]byte(cleaned), &p)
		if err != nil || p.Title == "" {
			p.Title = "새로운 소식 (파싱 에러)"
			p.Content = raw
		}

		tgTitle := fmt.Sprintf("🚀 [%s] 새 글 승인 (%s)", agent.Name(), topic)
		tgBody := fmt.Sprintf("📌 제목: %s\n\n📝 내용:\n%s", p.Title, p.Content)
		
		action, err := ui.Confirm(ctx, tgTitle, tgBody)
		if err == nil && action == ports.ActionApprove {
			finalPayload, _ := json.Marshal(map[string]string{
				"title": p.Title, "content": p.Content, "submadang": p.Submadang,
			})
			if err := agent.CreatePost(ctx, domain.Post{Content: string(finalPayload), Source: agent.Name()}); err == nil {
				store.IncrementPostCount(agent.Name(), today, time.Now().Unix())
			}
		}
	}
}