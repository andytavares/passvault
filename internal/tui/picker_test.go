package tui

import (
	"fmt"
	"strings"
	"testing"
)

const pickerVaultsJSON = `[
	{"id":"pd","title":"PagerDuty API Key","vault":{"name":"Private"}},
	{"id":"gh","title":"GitHub PAT","vault":{"name":"Employee"}},
	{"id":"aws","title":"AWS Prod","vault":{"name":"Employee"}}
]`

// openPicker switches to Mapping, presses n and applies the item list.
func openPicker(t *testing.T, m Model) Model {
	t.Helper()
	m = update(t, m, press("shift+tab"))
	m = update(t, m, press("n"))
	return update(t, m, itemsCmd(m.deps.Items)())
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = update(t, m, press(string(r)))
	}
	return m
}

func TestPickerSearchFiltersByTitle(t *testing.T) {
	f := newFixture(t)
	f.env.SetOpItems(t, pickerVaultsJSON)
	m := openPicker(t, f.loadModel(t))
	m = typeText(t, m, "gIt")
	view := viewText(m)
	if !strings.Contains(view, "GitHub PAT") || strings.Contains(view, "PagerDuty") || strings.Contains(view, "AWS Prod") {
		t.Fatalf("search gIt should list only GitHub PAT:\n%s", view)
	}
	m = update(t, m, press("backspace"))
	if m.pickerQuery != "gI" {
		t.Errorf("backspace left query %q, want gI", m.pickerQuery)
	}
}

func TestPickerSearchPicksFilteredItem(t *testing.T) {
	f := newFixture(t)
	f.env.SetOpItems(t, pickerVaultsJSON)
	writeFile(t, f.env.Root+"/op-item-aws.json", `{"id":"aws","title":"AWS Prod","vault":{"name":"Employee"},
		"fields":[{"label":"secret","reference":"op://Employee/AWS Prod/secret","type":"CONCEALED"}]}`)
	m := openPicker(t, f.loadModel(t))
	m = typeText(t, m, "aws")
	next, cmd := m.Update(press("enter"))
	m = update(t, next.(Model), cmd())
	m = update(t, m, press("enter"))
	if m.prompt != promptAddName || m.input != "aws-prod" || m.pickerRef != "op://Employee/AWS Prod/secret" {
		t.Fatalf("picked %q ref %q, want aws-prod from the filtered list", m.input, m.pickerRef)
	}
}

func TestPickerVaultFilterCycles(t *testing.T) {
	f := newFixture(t)
	f.env.SetOpItems(t, pickerVaultsJSON)
	m := openPicker(t, f.loadModel(t))
	m = update(t, m, press("right"))
	view := viewText(m)
	if m.active != viewMapping {
		t.Fatal("right must cycle vaults, not switch tabs, while the picker is open")
	}
	if !strings.Contains(view, "Employee") || strings.Contains(view, "PagerDuty") || !strings.Contains(view, "AWS Prod") {
		t.Fatalf("first vault should be Employee:\n%s", view)
	}
	m = typeText(t, m, "q")
	if m.pickerQuery != "q" || strings.Contains(viewText(m), "GitHub") {
		t.Fatalf("q should search, not quit; query %q", m.pickerQuery)
	}
}

func TestPickerVaultFilterWraps(t *testing.T) {
	f := newFixture(t)
	f.env.SetOpItems(t, pickerVaultsJSON)
	m := openPicker(t, f.loadModel(t))
	m = update(t, m, press("right"))
	m = update(t, m, press("right"))
	if view := viewText(m); !strings.Contains(view, "PagerDuty") || strings.Contains(view, "GitHub") {
		t.Fatalf("second vault should be Private:\n%s", view)
	}
	m = update(t, m, press("right"))
	if m.pickerVault != "" {
		t.Errorf("third right should wrap to all vaults, got %q", m.pickerVault)
	}
	m = update(t, m, press("left"))
	if m.pickerVault != "Private" {
		t.Errorf("left from all should go to the last vault, got %q", m.pickerVault)
	}
}

func TestPickerScrollsToCursor(t *testing.T) {
	f := newFixture(t)
	var entries []string
	for i := range 100 {
		entries = append(entries, fmt.Sprintf(`{"id":"i%d","title":"Item %03d","vault":{"name":"Private"}}`, i, i))
	}
	f.env.SetOpItems(t, "["+strings.Join(entries, ",")+"]")
	m := openPicker(t, f.loadModel(t))
	for range 60 {
		m = update(t, m, press("down"))
	}
	view := viewText(m)
	if !strings.Contains(view, "▸ Item 060") {
		t.Fatalf("cursor row should be visible:\n%s", view)
	}
	if strings.Contains(view, "Item 000") {
		t.Errorf("rows above the window should scroll off:\n%s", view)
	}
}
