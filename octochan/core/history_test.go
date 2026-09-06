package core

import (
	"strings"
	"testing"
	"time"
)

func TestPlanFromDiffIgnoresRemovals(t *testing.T) {
	// patch переносит значения, а не удаляет их: параметр, которого нет
	// в источнике изменений, в план попадать не должен.
	base := ConfigMap{"": {"max_connections": "100", "только_в_базе": "1"}}
	changes := ConfigMap{"": {"max_connections": "200", "новый": "да"}}

	plan := PlanFromDiff(CompareConfigs(base, changes, "база", "изменения"), "out.conf")

	byName := make(map[string]Change)
	for _, change := range plan.Changes {
		byName[change.Name] = change
	}

	if _, ok := byName["только_в_базе"]; ok {
		t.Error("параметр, отсутствующий в изменениях, попал в план")
	}
	if got := byName["max_connections"]; got.Kind != ChangeSet || got.From != "100" || got.To != "200" {
		t.Errorf("max_connections: %+v", got)
	}
	if got := byName["новый"]; got.Kind != ChangeAdd || got.To != "да" {
		t.Errorf("новый: %+v", got)
	}
}

func TestPlanIgnoresEquivalentValues(t *testing.T) {
	// Значения, одинаковые по смыслу, изменением не являются.
	base := ConfigMap{"": {"shared_buffers": "8GB"}}
	changes := ConfigMap{"": {"shared_buffers": "8192MB"}}

	plan := PlanFromDiff(CompareConfigs(base, changes, "a", "b"), "out.conf")
	if plan.Len() != 0 {
		t.Errorf("8GB и 8192MB попали в план: %+v", plan.Changes)
	}
}

func TestApplyPlanRoundTrip(t *testing.T) {
	base := ConfigMap{"": {"max_connections": "100"}, "server": {"port": "5432"}}
	changes := ConfigMap{"": {"max_connections": "200"}, "server": {"port": "5433"}}

	plan := PlanFromDiff(CompareConfigs(base, changes, "a", "b"), "out.conf")
	result := ApplyPlan(base, plan)

	if got := result[""]["max_connections"]; got != "200" {
		t.Errorf("max_connections = %q", got)
	}
	if got := result["server"]["port"]; got != "5433" {
		t.Errorf("server.port = %q", got)
	}
}

func TestPlanSortIsStable(t *testing.T) {
	plan := &Plan{Changes: []Change{
		{Target: "srv2", Name: "b"},
		{Target: "srv1", Name: "z"},
		{Target: "srv1", Name: "a"},
	}}
	plan.Sort()

	want := []string{"srv1/a", "srv1/z", "srv2/b"}
	for i, change := range plan.Changes {
		if got := change.Target + "/" + change.Name; got != want[i] {
			t.Errorf("позиция %d: %s, ожидалось %s", i, got, want[i])
		}
	}
}

func TestHistoryAppendAndList(t *testing.T) {
	store, err := NewHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Записи создаются с разным временем, чтобы порядок был однозначен.
	for i, summary := range []string{"первая", "вторая", "третья"} {
		entry := &Entry{
			Kind:      EntryDiff,
			Command:   "diff",
			Summary:   summary,
			Timestamp: time.Date(2026, 1, 1, 10, i, 0, 0, time.UTC),
		}
		if err := store.Append(entry); err != nil {
			t.Fatal(err)
		}
		if entry.ID == "" {
			t.Fatal("идентификатор не присвоен")
		}
		if entry.Author == "" {
			t.Fatal("автор не проставлен")
		}
	}

	// Список идёт от новых к старым.
	entries, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("получено %d записей", len(entries))
	}
	if entries[0].Summary != "третья" || entries[2].Summary != "первая" {
		t.Errorf("порядок нарушен: %s ... %s", entries[0].Summary, entries[2].Summary)
	}

	limited, err := store.List(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 || limited[0].Summary != "третья" {
		t.Errorf("limit не сработал: %d записей", len(limited))
	}
}

func TestHistoryGetByPrefix(t *testing.T) {
	store, err := NewHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	entry := &Entry{Kind: EntryPatch, Command: "patch", Summary: "тест"}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}

	// Полный идентификатор набирать неудобно, поэтому принимается префикс.
	found, err := store.Get(entry.ID[:4])
	if err != nil {
		t.Fatalf("поиск по префиксу: %v", err)
	}
	if found.ID != entry.ID {
		t.Errorf("найдена запись %s вместо %s", found.ID, entry.ID)
	}

	if _, err := store.Get("нет-такой"); err == nil {
		t.Error("несуществующая запись нашлась")
	}
}

func TestHistoryAmbiguousPrefixIsError(t *testing.T) {
	store, err := NewHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// Одинаковый префикс у двух записей не должен разрешаться наугад.
	for _, id := range []string{"abcd1111", "abcd2222"} {
		if err := store.Append(&Entry{ID: id, Kind: EntryDiff, Summary: id}); err != nil {
			t.Fatal(err)
		}
	}

	_, err = store.Get("abcd")
	if err == nil {
		t.Fatal("неоднозначный префикс разрешился")
	}
	if !strings.Contains(err.Error(), "нескольким") {
		t.Errorf("непонятное сообщение: %v", err)
	}
}

func TestHistoryMarkReverted(t *testing.T) {
	store, err := NewHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	entry := &Entry{
		Kind:       EntryPatch,
		Command:    "patch",
		Target:     "out.conf",
		Summary:    "изменено параметров: 1",
		Before:     ConfigMap{"": {"max_connections": "100"}},
		Reversible: true,
	}
	if err := store.Append(entry); err != nil {
		t.Fatal(err)
	}
	if entry.Reverted() {
		t.Error("свежая запись помечена отменённой")
	}

	if err := store.MarkReverted(entry, "откат1"); err != nil {
		t.Fatal(err)
	}

	// Пометка должна пережить перечитывание с диска.
	reloaded, err := store.Get(entry.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Reverted() || reloaded.RevertedBy != "откат1" {
		t.Errorf("пометка не сохранилась: %+v", reloaded)
	}
	if got := reloaded.Before[""]["max_connections"]; got != "100" {
		t.Errorf("состояние до операции потерялось: %q", got)
	}
}

func TestHistoryIDsAreUnique(t *testing.T) {
	// Прежние снапшоты брали UnixNano и давали коллизии при создании
	// нескольких записей подряд.
	store, err := NewHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[string]bool)
	for i := 0; i < 200; i++ {
		entry := &Entry{Kind: EntryDiff, Command: "diff", Summary: "x"}
		if err := store.Append(entry); err != nil {
			t.Fatal(err)
		}
		if seen[entry.ID] {
			t.Fatalf("повтор идентификатора %s на шаге %d", entry.ID, i)
		}
		seen[entry.ID] = true
	}
}

func TestHistoryEmptyDir(t *testing.T) {
	store, err := NewHistory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	entries, err := store.List(0)
	if err != nil {
		t.Fatalf("пустая история вернула ошибку: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("в пустой истории нашлось %d записей", len(entries))
	}
}
