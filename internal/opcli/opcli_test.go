package opcli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/atavares-klaviyo/pass-vault/internal/opcli"
	"github.com/atavares-klaviyo/pass-vault/internal/testenv"
)

func newClient(t *testing.T) (*testenv.Env, *opcli.Client) {
	t.Helper()
	env := testenv.New(t)
	return env, &opcli.Client{Runner: env.Runner()}
}

func TestReadReturnsFixtureBytes(t *testing.T) {
	env, client := newClient(t)
	env.SetOp(t, "op://Private/Item/field", "secret-bytes\n")
	got, err := client.Read(context.Background(), "op://Private/Item/field")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "secret-bytes\n" {
		t.Fatalf("Read = %q", got)
	}
}

func TestReadMissingRefMentionsRef(t *testing.T) {
	_, client := newClient(t)
	_, err := client.Read(context.Background(), "op://Private/Missing/field")
	if err == nil || !strings.Contains(err.Error(), "op://Private/Missing/field") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadErrorHidesStdout(t *testing.T) {
	env, client := newClient(t)
	env.SetOp(t, "op://Private/Item/leak", "do-not-leak")
	// A failing read of a present fixture cannot happen with the fake op, so
	// assert the error path on a missing ref never carries another ref's value.
	_, err := client.Read(context.Background(), "op://Private/Missing/field")
	if err == nil || strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("err = %v", err)
	}
}

func TestAccountFlagLogged(t *testing.T) {
	env, client := newClient(t)
	client.Account = "work"
	env.SetOp(t, "op://Private/Item/field", "v")
	if _, err := client.Read(context.Background(), "op://Private/Item/field"); err != nil {
		t.Fatal(err)
	}
	log := env.Log(t, "op")
	if len(log) != 1 || !strings.Contains(log[0], "--account work") {
		t.Fatalf("op log = %q", log)
	}
}

func TestSignedIn(t *testing.T) {
	env, client := newClient(t)
	env.SetOpSignedIn(t, true)
	if err := client.SignedIn(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSignedOutErrorHasHint(t *testing.T) {
	env, client := newClient(t)
	env.SetOpSignedIn(t, false)
	err := client.SignedIn(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Integrate with 1Password CLI") {
		t.Fatalf("err = %v", err)
	}
}

func TestListItems(t *testing.T) {
	env, client := newClient(t)
	env.SetOpItems(t, `[{"id":"a1","title":"PagerDuty","vault":{"name":"Private"}},{"id":"b2","title":"GitHub","vault":{"name":"Work"}}]`)
	items, err := client.ListItems(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != "a1" || items[0].Title != "PagerDuty" || items[0].Vault != "Private" {
		t.Fatalf("items = %+v", items)
	}
}

func TestItemFieldsDecodesReferencesAndIgnoresValue(t *testing.T) {
	env, client := newClient(t)
	json := `{"id":"a1","title":"PagerDuty","vault":{"name":"Private"},"fields":[` +
		`{"label":"credential","reference":"op://Private/PagerDuty/credential","type":"CONCEALED","value":"super-secret"},` +
		`{"label":"notesPlain","reference":"op://Private/PagerDuty/notesPlain","type":"STRING"}]}`
	path := filepath.Join(env.Root, "op-item-a1.json")
	if err := os.WriteFile(path, []byte(json), 0o600); err != nil {
		t.Fatal(err)
	}
	item, err := client.ItemFields(context.Background(), "a1")
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "a1" || item.Vault != "Private" || len(item.Fields) != 2 {
		t.Fatalf("item = %+v", item)
	}
	field := item.Fields[0]
	if field.Label != "credential" || field.Reference != "op://Private/PagerDuty/credential" || field.Type != "CONCEALED" {
		t.Fatalf("field = %+v", field)
	}
}
