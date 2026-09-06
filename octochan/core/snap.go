package core

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot фиксирует расхождение между двумя конфигами в момент времени.
type Snapshot struct {
	ID        string    `json:"id"`
	ParentID  *string   `json:"parent_id,omitempty"`
	Patch     *Diff     `json:"patch"`
	Author    string    `json:"author"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message,omitempty"`
}

// CreateSnapshot собирает снапшот из уже рассчитанного diff.
// Раньше функция сама вызывала CompareConfigs, дублируя работу вызывающего кода.
func CreateSnapshot(parentID *string, diff *Diff, author, message string) (*Snapshot, error) {
	if diff == nil {
		return nil, fmt.Errorf("не передан diff для снапшота")
	}

	id, err := generateSnapshotID()
	if err != nil {
		return nil, err
	}

	return &Snapshot{
		ID:        id,
		ParentID:  parentID,
		Patch:     diff,
		Author:    author,
		Timestamp: time.Now(),
		Message:   message,
	}, nil
}

// SaveSnapshot записывает снапшот в директорию snapshotsDir.
func SaveSnapshot(snapshot *Snapshot, snapshotsDir string) error {
	if err := os.MkdirAll(snapshotsDir, dirPerm); err != nil {
		return fmt.Errorf("не удалось создать директорию снапшотов: %w", err)
	}

	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return fmt.Errorf("не удалось сериализовать снапшот: %w", err)
	}

	path := filepath.Join(snapshotsDir, fmt.Sprintf("snapshot_%s.json", snapshot.ID))
	if err := os.WriteFile(path, data, filePerm); err != nil {
		return fmt.Errorf("не удалось записать снапшот: %w", err)
	}
	return nil
}

// LoadSnapshot читает снапшот по идентификатору.
func LoadSnapshot(snapshotID, snapshotsDir string) (*Snapshot, error) {
	path := filepath.Join(snapshotsDir, fmt.Sprintf("snapshot_%s.json", snapshotID))

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("не удалось прочитать снапшот %s: %w", snapshotID, err)
	}

	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return nil, fmt.Errorf("не удалось разобрать снапшот %s: %w", snapshotID, err)
	}
	return &snapshot, nil
}

// ApplySnapshot переносит в targetConfig значения левой стороны снапшота.
func ApplySnapshot(targetConfig ConfigMap, snapshot *Snapshot) (ConfigMap, error) {
	if snapshot == nil || snapshot.Patch == nil {
		return nil, fmt.Errorf("снапшот пуст")
	}
	if targetConfig == nil {
		targetConfig = make(ConfigMap)
	}

	for _, entry := range snapshot.Patch.Entries {
		if !entry.LeftExists {
			continue
		}
		if _, exists := targetConfig[entry.Section]; !exists {
			targetConfig[entry.Section] = make(map[string]string)
		}
		targetConfig[entry.Section][entry.Key] = entry.Left
	}

	return targetConfig, nil
}

// generateSnapshotID возвращает идентификатор вида <unix-время>-<случайность>.
// Прежний UnixNano давал коллизии при создании снапшотов в одном цикле.
func generateSnapshotID() (string, error) {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("не удалось сгенерировать идентификатор снапшота: %w", err)
	}
	return fmt.Sprintf("%d-%s", time.Now().Unix(), hex.EncodeToString(buf)), nil
}
