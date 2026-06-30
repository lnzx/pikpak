package pool

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lnzx/pikpak/internal/config"
)

// QuotaSnapshot is a cached quota state for one account.
type QuotaSnapshot struct {
	CloudDownloadLimit int64     `json:"cloud_download_limit"`
	CloudDownloadUsage int64     `json:"cloud_download_usage"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// AccountState tracks offline tasks and a quota snapshot for one account.
type AccountState struct {
	TaskIDs     []string       `json:"task_ids"`
	QuotaCache  *QuotaSnapshot `json:"quota_cache,omitempty"`
}

// TaskState is the persisted mapping from account alias to its state.
type TaskState map[string]*AccountState

func statePath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "task_state.json"), nil
}

var stateMu sync.Mutex

// LoadState reads task_state.json. Returns an empty state if the file does not exist.
func LoadState() (TaskState, error) {
	stateMu.Lock()
	defer stateMu.Unlock()

	p, err := statePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return TaskState{}, nil
		}
		return nil, err
	}
	var s TaskState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s == nil {
		s = TaskState{}
	}
	return s, nil
}

// SaveState atomically writes the task state to disk.
func SaveState(s TaskState) error {
	stateMu.Lock()
	defer stateMu.Unlock()

	p, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// GetOrCreate returns the AccountState for alias, creating it if absent.
func (s TaskState) GetOrCreate(alias string) *AccountState {
	if a, ok := s[alias]; ok {
		return a
	}
	a := &AccountState{}
	s[alias] = a
	return a
}

// AddTask records a task ID under the given account.
func (s TaskState) AddTask(alias, taskID string) {
	s.GetOrCreate(alias).TaskIDs = append(s.GetOrCreate(alias).TaskIDs, taskID)
}

// RemoveTask removes a task ID from the given account.
func (s TaskState) RemoveTask(alias, taskID string) {
	acc := s[alias]
	if acc == nil {
		return
	}
	filtered := make([]string, 0, len(acc.TaskIDs))
	for _, id := range acc.TaskIDs {
		if id != taskID {
			filtered = append(filtered, id)
		}
	}
	acc.TaskIDs = filtered
}

// ClearTasks removes all task IDs for the given account.
func (s TaskState) ClearTasks(alias string) {
	if acc := s[alias]; acc != nil {
		acc.TaskIDs = nil
	}
}

// FindTaskOwner returns the account alias that owns the given task ID.
func (s TaskState) FindTaskOwner(taskID string) string {
	for alias, acc := range s {
		for _, id := range acc.TaskIDs {
			if id == taskID {
				return alias
			}
		}
	}
	return ""
}

// AccountsWithTasks returns aliases that have at least one recorded task.
func (s TaskState) AccountsWithTasks() []string {
	var out []string
	for alias, acc := range s {
		if len(acc.TaskIDs) > 0 {
			out = append(out, alias)
		}
	}
	return out
}
