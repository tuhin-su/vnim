package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tuhin-su/vnim/pkg/config"
)

const StateDir = "/var/run/vnim/plans"

type PlanState struct {
	Name      string           `json:"name"`
	YAMLPath  string           `json:"yaml_path"`
	CreatedAt time.Time        `json:"created_at"`
	Objects   []config.Object  `json:"objects"`
	Services  []ActiveService  `json:"services"`
}

type ActiveService struct {
	Type      string `json:"type"`
	Interface string `json:"interface"`
	PID       int    `json:"pid"`
}

func GetStatePath(planName string) string {
	return filepath.Join(StateDir, fmt.Sprintf("%s.json", planName))
}

func SaveState(planName string, ps *PlanState) error {
	if err := os.MkdirAll(StateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	data, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal plan state: %w", err)
	}

	statePath := GetStatePath(planName)
	if err := os.WriteFile(statePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

func LoadState(planName string) (*PlanState, error) {
	statePath := GetStatePath(planName)
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	var ps PlanState
	if err := json.Unmarshal(data, &ps); err != nil {
		return nil, fmt.Errorf("failed to unmarshal plan state: %w", err)
	}

	return &ps, nil
}

func DeleteState(planName string) error {
	statePath := GetStatePath(planName)
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}

	planDir := filepath.Join(StateDir, planName)
	if err := os.RemoveAll(planDir); err != nil {
		return fmt.Errorf("failed to delete plan subdirectory: %w", err)
	}

	return nil
}

func ListStates() ([]*PlanState, error) {
	if err := os.MkdirAll(StateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create state directory: %w", err)
	}

	entries, err := os.ReadDir(StateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var states []*PlanState
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		planName := entry.Name()[:len(entry.Name())-len(".json")]
		ps, err := LoadState(planName)
		if err == nil {
			states = append(states, ps)
		}
	}

	return states, nil
}
