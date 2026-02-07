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
	fmt.Println("🤖 d3k Integrated Agent Starting... [v1.3.7-Verbose-Logs]")

	ctx := context.Background()
	var store ports.Storage
	var err error

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL != "" {
		store, err = storage.NewPostgresStorage(ctx, dbURL)
		if err == nil { 
			fmt.Println("🐘 Storage: PostgreSQL Connected") 
		} else {
			fmt.Printf("⚠️  DB Connection failed: %v\n", err)
		}
	}
	if store == nil {
		store, _ = storage.NewJSONStorage("data/storage.json")
		fmt.Println("📄 Storage: JSON File Mode")
	}

	myBrain, _ := brain.NewGeminiBrain(ctx, os.Getenv("GEMINI_API_KEY"))
	if myBrain != nil { fmt.Println("🧠 Brain: Gemini Ready") }

	ui, _ := telegram.NewTelegramUI(os.Getenv("TELEGRAM_BOT_TOKEN"), os.Getenv("TELEGRAM_CHAT_ID"))
	if ui != nil { fmt.Println("📲 UI: Telegram Connected") }

	agents := []ports.Site{
		botmadang.NewClient(store),
	}
	for _, agent := range agents { 
		if err := agent.Initialize(ctx); err != nil {
			fmt.Printf("❌ [%s] Init Failed: %v\n", agent.Name(), err)
		}
	}

	trigger := make(chan bool, 1)
	go func() {
		reader := bufio.NewReader(os.Stdin)
		for {
			reader.ReadString('\n')
			trigger <- true
		}
	}()

	fmt.Println("🚀 System ready. Listening for activities...")

	firstRun := true
	for {
		fmt.Printf("\n--- 🔄 Check Cycle (%s) ---\n", time.Now().Format("15:04:05"))
		for _, agent := range agents {
			processAgent(ctx, agent, myBrain, ui, store, firstRun)
		}
		firstRun = false

		fmt.Println("\nWaiting 10 minutes... (Press Enter to trigger)")
		select {
		case <-time.After(10 * time.Minute):
		case <-trigger:
			fmt.Println("⚡ Manual trigger received!")
		}
	}
}

func processAgent(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage, firstRun bool) {
	fmt.Printf("[%s] Status Update:\n", agent.Name())
	
	fmt.Print("  🔔 Notifs: ")
	handleNotifications(ctx, agent, brain, ui, store)
	
	fmt.Print("  🌟 Proactive: ")
	handleProactiveCommenting(ctx, agent, brain, ui, store)
	
	fmt.Print("  📝 Posting: ")
	handleDailyPosting(ctx, agent, brain, ui, store, firstRun)
	
	fmt.Print("  🧠 Learning: ")
	learnFromCommunity(ctx, agent, brain, store)
}

func learnFromCommunity(ctx context.Context, agent ports.Site, brain ports.Brain, store ports.Storage) {
	posts, err := agent.GetRecentPosts(ctx, 3)
	if err != nil { fmt.Printf("Error: %v\n", err); return }
	
	learned := 0
	for _, p := range posts {
		insightText, err := brain.SummarizeInsight(ctx, p)
		if err == nil && insightText != "" {
			store.SaveInsight(ctx, domain.Insight{PostID: p.ID, Source: agent.Name(), Topic: p.Title, Content: insightText})
			learned++
		}
	}
	fmt.Printf("%d new items learned.\n", learned)
}

func handleNotifications(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, _, _ := store.GetCommentStats(agent.Name())
	if count >= 20 { 
		fmt.Printf("Daily limit reached (%d/20).\n", count)
		return 
	}

	notifs, err := agent.GetNotifications(ctx, true)
	if err != nil { fmt.Printf("Error: %v\n", err); return }
	if len(notifs) == 0 { 
		fmt.Println("0 unread notifications.")
		return 
	}

	groups := make(map[string]struct{ title, latestCID, postID string; contents, notifIDs []string })
	for _, n := range notifs {
		if n.Type != "comment_on_post" && n.Type != "reply_to_comment" { continue }
		g := groups[n.PostID]; g.title = n.PostTitle; g.latestCID = n.CommentID; g.postID = n.PostID
		g.contents = append(g.contents, fmt.Sprintf("- %s: %s", n.ActorName, n.Content))
		g.notifIDs = append(g.notifIDs, n.ID)
		groups[n.PostID] = g
	}

	if len(groups) == 0 {
		fmt.Println("No actionable comment notifications.")
		return
	}

	fmt.Printf("Found %d threads to reply.\n", len(groups))
	for pid, g := range groups {
		if brain == nil || ui == nil || count >= 20 { break }
		peerText := strings.Join(g.contents, "\n")
		reply, err := brain.GenerateReply(ctx, g.title, peerText)
		if err != nil { fmt.Printf("    ❌ Brain failed: %v\n", err); continue }

		summary, _ := brain.SummarizeInsight(ctx, domain.Post{Content: peerText})
		
		tgTitle := fmt.Sprintf("💬 [%s] 답글 승인", agent.Name())
		tgBody := fmt.Sprintf("📍 글: %s\n📄 요약: %s\n\n🤖 답글: %s", g.title, summary, reply)
		
		action, err := ui.Confirm(ctx, tgTitle, tgBody)
		if err == nil && action == ports.ActionApprove {
			if err := agent.ReplyToComment(ctx, pid, g.latestCID, reply); err == nil {
				for _, nid := range g.notifIDs { agent.MarkNotificationRead(ctx, nid) }
				store.IncrementCommentCount(agent.Name(), today)
				count++
				fmt.Println("    ✅ Approved and Sent.")
			}
		} else {
			fmt.Println("    ⏩ Skipped/Rejected.")
		}
	}
}

func handleProactiveCommenting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage) {
	today := time.Now().Format("2006-01-02")
	count, _, _ := store.GetCommentStats(agent.Name())
	if count >= 20 { 
		fmt.Printf("Daily limit reached (%d/20).\n", count)
		return 
	}

	posts, err := agent.GetRecentPosts(ctx, 5)
	if err != nil { fmt.Printf("Error: %v\n", err); return }
	
	evaluated := 0
	for _, p := range posts {
		if done, _ := store.IsProactiveDone(agent.Name(), p.ID); done { continue }
		evaluated++
		score, reason, err := brain.EvaluatePost(ctx, p)
		if err != nil || score < 7 { continue }

		fmt.Printf("\n    ✨ High interest post (%dpt): %s\n", score, p.Title)
		reply, _ := brain.GenerateReply(ctx, p.Title, p.Content)
		summary, _ := brain.SummarizeInsight(ctx, p)

		tgTitle := fmt.Sprintf("🌟 [%s] 선제 댓글 (%d점)", agent.Name(), score)
		tgBody := fmt.Sprintf("📍 제목: %s\n📄 요약: %s\n\n🤖 댓글: %s\n💡 이유: %s", p.Title, summary, reply, reason)
		
		action, err := ui.Confirm(ctx, tgTitle, tgBody)
		if err == nil && action == ports.ActionApprove {
			if err := agent.CreateComment(ctx, p.ID, reply); err == nil {
				store.MarkProactive(agent.Name(), p.ID)
				store.IncrementCommentCount(agent.Name(), today)
				count++
				fmt.Println("    ✅ Approved and Sent.")
			}
		} else {
			store.MarkProactive(agent.Name(), p.ID)
			fmt.Println("    ⏩ Rejected and Marked as Done.")
		}
	}
	fmt.Printf("%d posts evaluated.\n", evaluated)
}

func handleDailyPosting(ctx context.Context, agent ports.Site, brain ports.Brain, ui ports.Interaction, store ports.Storage, firstRun bool) {
	today := time.Now().Format("2006-01-02")
	count, lastDate, lastTs, _ := store.GetPostStats(agent.Name())
	if lastDate != today { count = 0 }

	if count >= 4 {
		fmt.Printf("Daily limit reached (%d/4).\n", count)
		return
	}

	elapsed := time.Since(time.Unix(lastTs, 0))
	if lastTs != 0 && elapsed < 2*time.Hour {
		fmt.Printf("Cooldown (%.0f mins left).\n", 120-elapsed.Minutes())
		return
	}

	chance := rand.Float32()
	if !firstRun && chance > 0.4 {
		fmt.Printf("Probability skip (Roll: %.2f > 0.40).\n", chance)
		return
	}

	topics := []string{"금융 경제", "IT 기술", "일상 지혜", "커리어"}
	topic := topics[rand.Intn(len(topics))]
	fmt.Printf("Generating post about '%s'... ", topic)

	raw, err := brain.GeneratePost(ctx, topic)
	if err != nil { fmt.Printf("❌ AI Error: %v\n", err); return }

	// JSON 파싱 안정화
	cleaned := raw
	if start := strings.Index(raw, "{"); start != -1 {
		if end := strings.LastIndex(raw, "}"); end != -1 && end > start {
			cleaned = raw[start : end+1]
		}
	}
	var p struct { Title string `json:"title"`; Content string `json:"content"`; Sub string `json:"submadang"` }
	if err := json.Unmarshal([]byte(cleaned), &p); err != nil {
		fmt.Printf("❌ Parse Error. ")
		p.Title = "새로운 디지털 소식"; p.Content = raw
	}

	tgTitle := fmt.Sprintf("🚀 [%s] 새 글 승인", agent.Name())
	tgBody := fmt.Sprintf("📌 제목: %s\n\n📝 내용:\n%s", p.Title, p.Content)
	
	action, err := ui.Confirm(ctx, tgTitle, tgBody)
	if err == nil && action == ports.ActionApprove {
		final, _ := json.Marshal(map[string]string{"title": p.Title, "content": p.Content, "submadang": p.Sub})
		if err := agent.CreatePost(ctx, domain.Post{Content: string(final), Source: agent.Name()}); err == nil {
			store.IncrementPostCount(agent.Name(), today, time.Now().Unix())
			fmt.Println("✅ Success.")
		}
	} else {
		fmt.Println("⏩ Rejected.")
	}
}
