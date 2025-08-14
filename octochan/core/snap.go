package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Snapshot struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Patch     string    `json:"patch"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
}

func CreateSnapshot(parentID *string, db1Config, db2Config map[string]map[string]string, author, message string) (*Snapshot, error) {
	diff := CompareConfigs(db1Config, db2Config, "source", "target")
	patch, err := json.Marshal(diff)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal diff: %v", err)
	}

	return &Snapshot{
		ID:        generateSnapshotID(),
		ParentID:  parentID,
		Patch:     string(patch),
		Author:    author,
		Timestamp: time.Now(),
		Message:   message,
	}, nil
}
func SaveSnapshot(snapshot *Snapshot, snapshotsDir string) error {

	if err := os.MkdirAll(snapshotsDir, 0755); err != nil {
		return fmt.Errorf("failed to create snapshots directory: %v", err)
	}

	filePath := filepath.Join(snapshotsDir, fmt.Sprintf("snapshot_%s.json", snapshot.ID))
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create snapshot file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		return fmt.Errorf("failed to encode snapshot: %v", err)
	}

	return nil
}

func LoadSnapshot(snapshotID, snapshotsDir string) (*Snapshot, error) {
	filePath := filepath.Join(snapshotsDir, fmt.Sprintf("snapshot_%s.json", snapshotID))
	file, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read snapshot file: %v", err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(file, &snapshot); err != nil {
		return nil, fmt.Errorf("failed to decode snapshot: %v", err)
	}

	return &snapshot, nil
}

func ApplySnapshot(targetConfig map[string]map[string]string, snapshot *Snapshot) (map[string]map[string]string, error) {

	var diff map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(snapshot.Patch), &diff); err != nil {
		return nil, fmt.Errorf("failed to unmarshal patch: %v", err)
	}

	for param, data := range diff {
		if sourceVal, ok := data["source"]; ok && sourceVal != "N/A" {

			parts := strings.SplitN(param, ".", 2)
			if len(parts) != 2 {
				continue
			}

			section := parts[0]
			key := parts[1]

			if _, exists := targetConfig[section]; !exists {
				targetConfig[section] = make(map[string]string)
			}

			targetConfig[section][key] = fmt.Sprintf("%v", sourceVal)
		}
	}

	return targetConfig, nil
}

func generateSnapshotID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
