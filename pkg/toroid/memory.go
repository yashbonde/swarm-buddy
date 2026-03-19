package toroid

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"google.golang.org/genai"
)

type MemoryStore struct {
	sessionID   string
	sessionPath string
	titlePath   string
	memoryPath  string
	usagePath   string
	compactPath string
	prevPath    string
	mu          sync.Mutex
}

func swbDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".swb"), nil
}

func newMemoryStore(sessionID string) (*MemoryStore, error) {
	base, err := swbDir()
	if err != nil {
		return nil, err
	}

	for _, dir := range []string{
		filepath.Join(base, "sessions"),
		filepath.Join(base, "memories"),
		filepath.Join(base, "tasks"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	return &MemoryStore{
		sessionID:   sessionID,
		sessionPath: filepath.Join(base, "sessions", sessionID+".jsonl"),
		titlePath:   filepath.Join(base, "sessions", sessionID+".title"),
		memoryPath:  filepath.Join(base, "memories", sessionID+".json"),
		usagePath:   filepath.Join(base, "sessions", sessionID+".usage.jsonl"),
		compactPath: filepath.Join(base, "sessions", sessionID+".compact.md"),
		prevPath:    filepath.Join(base, "sessions", sessionID+".prev"),
	}, nil
}

// SaveTitle writes the session title derived from the first prompt.
func (m *MemoryStore) SaveTitle(title string) error {
	return os.WriteFile(m.titlePath, []byte(title), 0644)
}

// LoadTitle reads the session title.
func (m *MemoryStore) LoadTitle() string {
	b, err := os.ReadFile(m.titlePath)
	if err != nil {
		return ""
	}
	return string(b)
}

// SessionInfo holds metadata for listing sessions.
type SessionInfo struct {
	ID    string
	Title string
}

// ListSessions returns all sessions sorted by modification time (newest first).
func ListSessions() ([]SessionInfo, error) {
	base, err := swbDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(filepath.Join(base, "sessions"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var sessions []SessionInfo
	for _, e := range entries {
		if filepath.Ext(e.Name()) != ".jsonl" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".jsonl")]
		titleBytes, _ := os.ReadFile(filepath.Join(base, "sessions", id+".title"))
		title := string(titleBytes)
		if title == "" {
			title = "(no title)"
		}
		sessions = append(sessions, SessionInfo{ID: id, Title: title})
	}
	// reverse so newest (lexicographically largest ID) is first
	for i, j := 0, len(sessions)-1; i < j; i, j = i+1, j-1 {
		sessions[i], sessions[j] = sessions[j], sessions[i]
	}
	return sessions, nil
}

// DeleteSession removes all files associated with a session ID.
func DeleteSession(id string) error {
	base, err := swbDir()
	if err != nil {
		return err
	}
	for _, path := range []string{
		filepath.Join(base, "sessions", id+".jsonl"),
		filepath.Join(base, "sessions", id+".title"),
		filepath.Join(base, "sessions", id+".usage.jsonl"),
		filepath.Join(base, "sessions", id+".compact.md"),
		filepath.Join(base, "sessions", id+".prev"),
		filepath.Join(base, "memories", id+".json"),
		filepath.Join(base, "tasks", id+".json"),
	} {
		os.Remove(path) // ignore not-found errors
	}
	return nil
}

// SaveCompact writes the compaction summary markdown for this session.
func (m *MemoryStore) SaveCompact(summary string) error {
	return os.WriteFile(m.compactPath, []byte(summary), 0644)
}

// SavePrevSession writes the previous session ID so the history chain is rebuildable.
func (m *MemoryStore) SavePrevSession(prevID string) error {
	return os.WriteFile(m.prevPath, []byte(prevID), 0644)
}

// LoadPrevSession returns the previous session ID, or empty string if none.
func (m *MemoryStore) LoadPrevSession() string {
	b, err := os.ReadFile(m.prevPath)
	if err != nil {
		return ""
	}
	return string(b)
}

// AppendTurnCost writes a single turn's cost (in paise) as a JSON line to the usage log.
func (m *MemoryStore) AppendTurnCost(turnPaise, totalPaise int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.OpenFile(m.usagePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(map[string]int64{"turn_paise": turnPaise, "total_paise": totalPaise})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", b)
	return err
}

// AppendUsage writes a single UsageMetadata snapshot as a JSON line to the usage log.
func (m *MemoryStore) AppendUsage(u *genai.GenerateContentResponseUsageMetadata) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.OpenFile(m.usagePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(u)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", b)
	return err
}

// Append writes a single Content as a JSON line to the session log.
func (m *MemoryStore) Append(content *genai.Content) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	f, err := os.OpenFile(m.sessionPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(content)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", b)
	return err
}

// History reads all Content lines from the session log.
func (m *MemoryStore) History() ([]*genai.Content, error) {
	f, err := os.Open(m.sessionPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var contents []*genai.Content
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for sc.Scan() {
		var c genai.Content
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, err
		}
		contents = append(contents, &c)
	}
	return contents, sc.Err()
}

// LoadMemories reads the agent's persistent memory file.
func (m *MemoryStore) LoadMemories() (map[string]any, error) {
	b, err := os.ReadFile(m.memoryPath)
	if os.IsNotExist(err) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, err
	}
	var mem map[string]any
	return mem, json.Unmarshal(b, &mem)
}

// SaveMemories writes the agent's persistent memory file.
func (m *MemoryStore) SaveMemories(mem map[string]any) error {
	b, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.memoryPath, b, 0644)
}
