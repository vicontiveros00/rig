package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MessageRecord struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

type ChatSession struct {
	ID        string          `json:"id"`
	Provider  string          `json:"provider"`
	Model     string          `json:"model"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	Messages  []MessageRecord `json:"messages"`
}

type ChatMeta struct {
	ID        string
	Model     string
	Provider  string
	CreatedAt time.Time
	Preview   string
}

type ScratchMeta struct {
	Filename  string
	Timestamp time.Time
	Preview   string
}

func historyDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rig", "history")
}

func chatDir() string    { return filepath.Join(historyDir(), "chat") }
func scratchDir() string { return filepath.Join(historyDir(), "scratch") }
func planDir() string    { return filepath.Join(historyDir(), "plan") }

func rigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".rig")
}

type Task struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"` // "pending", "in_progress", "done"
	Notes    string `json:"notes,omitempty"`
	Children []Task `json:"children,omitempty"`
}

type Plan struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tasks     []Task    `json:"tasks"`
}

type PlanMeta struct {
	ID         string
	Title      string
	CreatedAt  time.Time
	TaskCount  int
	DoneCount  int
}

func SaveChat(session ChatSession) error {
	dir := chatDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating chat history dir: %w", err)
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling chat session: %w", err)
	}

	path := filepath.Join(dir, session.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

func ListChats() ([]ChatMeta, error) {
	dir := chatDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metas []ChatMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var session ChatSession
		if err := json.Unmarshal(data, &session); err != nil {
			continue
		}

		preview := ""
		for _, m := range session.Messages {
			if m.Role == "user" {
				preview = m.Content
				if len(preview) > 80 {
					preview = preview[:77] + "..."
				}
				break
			}
		}

		metas = append(metas, ChatMeta{
			ID:        session.ID,
			Model:     session.Model,
			Provider:  session.Provider,
			CreatedAt: session.CreatedAt,
			Preview:   preview,
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].CreatedAt.After(metas[j].CreatedAt)
	})

	return metas, nil
}

func LoadChat(id string) (ChatSession, error) {
	path := filepath.Join(chatDir(), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return ChatSession{}, fmt.Errorf("reading chat session: %w", err)
	}

	var session ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return ChatSession{}, fmt.Errorf("parsing chat session: %w", err)
	}
	return session, nil
}

func GenerateChatID(model string) string {
	ts := time.Now().Format("2006-01-02T15-04-05")
	safe := strings.ReplaceAll(model, "/", "-")
	return ts + "_" + safe
}

func ArchiveScratch(content string) error {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	dir := scratchDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating scratch history dir: %w", err)
	}

	filename := time.Now().Format("2006-01-02T15-04-05") + ".md"
	path := filepath.Join(dir, filename)
	return os.WriteFile(path, []byte(content), 0o644)
}

func ListScratches() ([]ScratchMeta, error) {
	dir := scratchDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metas []ScratchMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		preview := ""
		content := string(data)
		if idx := strings.IndexByte(content, '\n'); idx > 0 {
			preview = content[:idx]
		} else if len(content) > 0 {
			preview = content
		}
		if len(preview) > 80 {
			preview = preview[:77] + "..."
		}

		name := strings.TrimSuffix(e.Name(), ".md")
		ts, _ := time.Parse("2006-01-02T15-04-05", name)

		metas = append(metas, ScratchMeta{
			Filename:  e.Name(),
			Timestamp: ts,
			Preview:   preview,
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].Timestamp.After(metas[j].Timestamp)
	})

	return metas, nil
}

func LoadScratch(filename string) (string, error) {
	path := filepath.Join(scratchDir(), filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading scratch: %w", err)
	}
	return string(data), nil
}

func SavePlan(p Plan) error {
	dir := planDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating plan history dir: %w", err)
	}

	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling plan: %w", err)
	}

	path := filepath.Join(dir, p.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

func ListPlans() ([]PlanMeta, error) {
	dir := planDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var metas []PlanMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}

		var p Plan
		if err := json.Unmarshal(data, &p); err != nil {
			continue
		}

		total, done := countTasks(p.Tasks)
		metas = append(metas, PlanMeta{
			ID:        p.ID,
			Title:     p.Title,
			CreatedAt: p.CreatedAt,
			TaskCount: total,
			DoneCount: done,
		})
	}

	sort.Slice(metas, func(i, j int) bool {
		return metas[i].CreatedAt.After(metas[j].CreatedAt)
	})

	return metas, nil
}

func CountTasksPublic(tasks []Task) (total, done int) {
	return countTasks(tasks)
}

func countTasks(tasks []Task) (total, done int) {
	for _, t := range tasks {
		total++
		if t.Status == "done" {
			done++
		}
		ct, cd := countTasks(t.Children)
		total += ct
		done += cd
	}
	return
}

func LoadPlan(id string) (Plan, error) {
	path := filepath.Join(planDir(), id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Plan{}, fmt.Errorf("reading plan: %w", err)
	}

	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, fmt.Errorf("parsing plan: %w", err)
	}
	return p, nil
}

func GeneratePlanID(title string) string {
	ts := time.Now().Format("2006-01-02T15-04-05")
	safe := strings.ReplaceAll(strings.ToLower(title), " ", "-")
	if len(safe) > 30 {
		safe = safe[:30]
	}
	return ts + "_" + safe
}

func SetActivePlan(id string) error {
	path := filepath.Join(rigDir(), "active_plan")
	return os.WriteFile(path, []byte(id), 0o644)
}

func GetActivePlan() (string, error) {
	path := filepath.Join(rigDir(), "active_plan")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}
