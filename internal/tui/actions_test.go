package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/mapping"
)

// typeKeys feeds one key per character, then enter; any returned command is
// run and its message applied, so mutations take effect.
func typeKeys(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = update(t, m, press(string(r)))
	}
	next, cmd := m.Update(press("enter"))
	m = next.(Model)
	if cmd != nil {
		if msg := cmd(); msg != nil {
			m = update(t, m, msg)
		}
	}
	return m
}

func TestDeleteRequiresExactName(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	f.env.Insert(t, "beta", "beta-value")
	m := f.loadModel(t)
	m = update(t, m, press("d"))
	if m.prompt != promptDelete {
		t.Fatalf("prompt = %v, want delete", m.prompt)
	}
	m = typeKeys(t, m, "beta")
	if !f.deps.Store.Exists("alpha") {
		t.Fatal("wrong name must not delete the entry")
	}
	m = update(t, m, press("d"))
	m = typeKeys(t, m, "alpha")
	if f.deps.Store.Exists("alpha") {
		t.Fatal("exact name should delete the entry")
	}
}

// openRollback presses R on the older revision and applies the plan.
func openRollback(t *testing.T, m Model) Model {
	t.Helper()
	next, cmd := m.Update(press("R"))
	m = next.(Model)
	if m.prompt != promptRollback {
		t.Fatalf("R should open the rollback prompt, got %v", m.prompt)
	}
	plan, ok := cmd().(planMsg)
	if !ok {
		t.Fatalf("plan command returned %T", cmd())
	}
	return update(t, m, plan)
}

func TestRollbackRestoresOlderValue(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "first-value")
	m := f.loadModel(t)
	f.env.Insert(t, "alpha", "second-value")
	m.revisions, _ = f.deps.Store.History(context.Background(), "", 10)
	m.revCursor = 1 // the older commit
	m.active = viewHistory
	m = openRollback(t, m)
	if !strings.Contains(m.rollbackPromptText(), "alpha") {
		t.Errorf("plan should name alpha:\n%s", m.rollbackPromptText())
	}
	m = typeKeys(t, m, "nope")
	if got := f.env.Show(t, "alpha"); got != "second-value" {
		t.Errorf("wrong word must not roll back; got %q", got)
	}
	m = openRollback(t, m)
	m = typeKeys(t, m, "rollback")
	if got := f.env.Show(t, "alpha"); got != "first-value" {
		t.Errorf("rollback should restore first-value; got %q", got)
	}
}

const pickerItemsJSON = `[{"id":"item1","title":"PagerDuty API Key","vault":{"name":"Private"}}]`

const pickerItemJSON = `{
	"id":"item1","title":"PagerDuty API Key","vault":{"name":"Private"},
	"fields":[{"label":"credential","reference":"op://Private/PagerDuty API Key/credential","type":"CONCEALED"}]
}`

// walkPicker drives the picker from item list to the env prompt.
func walkPicker(t *testing.T, m Model) Model {
	t.Helper()
	m = update(t, m, press("4"))
	m = update(t, m, press("a"))
	m = update(t, m, itemsCmd(m.deps.Items)())
	if len(m.pickerItems) != 1 {
		t.Fatalf("picker items = %d, want 1", len(m.pickerItems))
	}
	m = update(t, m, press("enter"))
	m = update(t, m, fieldsCmd(m.deps.Items, "item1")())
	if len(m.pickerFields) != 1 {
		t.Fatalf("picker fields = %d, want 1", len(m.pickerFields))
	}
	if strings.Contains(viewText(m), "secret-field-value") {
		t.Error("picker must never show field values")
	}
	m = update(t, m, press("enter"))
	if m.prompt != promptAddName || m.input != "pagerduty-api-key" {
		t.Fatalf("name prompt = %v %q", m.prompt, m.input)
	}
	m = update(t, m, press("enter"))
	if m.prompt != promptAddEnv {
		t.Fatalf("env prompt = %v", m.prompt)
	}
	return typeKeys(t, m, "PAGERDUTY_API_KEY")
}

func TestMappingAddPicker(t *testing.T) {
	f := newFixture(t)
	f.env.SetOpItems(t, pickerItemsJSON)
	writeFile(t, f.env.Root+"/op-item-item1.json", pickerItemJSON)
	m := f.loadModel(t)
	m = walkPicker(t, m)
	loaded, err := mapping.Load(f.deps.MappingPath)
	if err != nil {
		t.Fatal(err)
	}
	secret, ok := loaded.Find("pagerduty-api-key")
	if !ok {
		t.Fatal("AppendSecret did not write the entry")
	}
	if secret.Ref != "op://Private/PagerDuty API Key/credential" || secret.Env != "PAGERDUTY_API_KEY" {
		t.Errorf("appended %+v", secret)
	}
	if strings.Contains(viewText(m), "secret-field-value") {
		t.Error("view leaked a field value")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTooNarrowTerminal(t *testing.T) {
	f := newFixture(t)
	f.env.Insert(t, "alpha", "alpha-value")
	m := f.loadModel(t)
	m.width = 40
	if got := viewText(m); !strings.Contains(got, "terminal too narrow (need 60 columns)") {
		t.Errorf("narrow render = %q", got)
	}
}
