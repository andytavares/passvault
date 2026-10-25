// Package opcli wraps the 1Password CLI (op). Secrets pass only through
// stdout; item field values are never decoded.
package opcli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/atavares-klaviyo/pass-vault/internal/execx"
)

// Client talks to op for one optional account.
type Client struct {
	Runner  execx.Runner
	Account string
}

// Item is one 1Password item, without any field values.
type Item struct {
	ID     string
	Title  string
	Vault  string
	Fields []Field
}

// Field is one field of an Item. Value is deliberately absent.
type Field struct {
	Label     string
	Reference string
	Type      string
}

// opCmd builds an op command, adding --account when set.
func (c *Client) opCmd(args ...string) execx.Cmd {
	if c.Account != "" {
		args = append(args, "--account", c.Account)
	}
	return execx.Cmd{Name: "op", Args: args}
}

// Read returns the secret at ref (op://Vault/Item/field).
func (c *Client) Read(ctx context.Context, ref string) ([]byte, error) {
	result, err := c.Runner.Run(ctx, c.opCmd("read", "-n", ref))
	if err != nil {
		// op errors can echo argv; stderr names the ref, stdout may hold a
		// partial secret, so only stderr goes into the error.
		return nil, fmt.Errorf("op read %s: %s", ref, strings.TrimSpace(string(result.Stderr)))
	}
	return result.Stdout, nil
}

// SignedIn reports whether op can reach a 1Password account.
func (c *Client) SignedIn(ctx context.Context) error {
	if _, err := c.Runner.Run(ctx, c.opCmd("whoami")); err != nil {
		return fmt.Errorf("op whoami: %w\nOpen 1Password > Settings > Developer > Integrate with 1Password CLI, then retry", err)
	}
	return nil
}

type listEntry struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Vault struct {
		Name string `json:"name"`
	} `json:"vault"`
}

// ListItems returns every item op can see, without field values.
func (c *Client) ListItems(ctx context.Context) ([]Item, error) {
	result, err := c.Runner.Run(ctx, c.opCmd("item", "list", "--format", "json"))
	if err != nil {
		return nil, err
	}
	var entries []listEntry
	if err := json.Unmarshal(result.Stdout, &entries); err != nil {
		return nil, fmt.Errorf("op item list: %w", err)
	}
	items := make([]Item, len(entries))
	for i, entry := range entries {
		items[i] = Item{ID: entry.ID, Title: entry.Title, Vault: entry.Vault.Name}
	}
	return items, nil
}

type itemDocument struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Vault struct {
		Name string `json:"name"`
	} `json:"vault"`
	Fields []struct {
		Label     string `json:"label"`
		Reference string `json:"reference"`
		Type      string `json:"type"`
	} `json:"fields"`
}

// ItemFields returns item id with its field labels, references and types.
// The JSON's field values are never decoded.
func (c *Client) ItemFields(ctx context.Context, id string) (Item, error) {
	result, err := c.Runner.Run(ctx, c.opCmd("item", "get", id, "--format", "json"))
	if err != nil {
		return Item{}, err
	}
	var document itemDocument
	if err := json.Unmarshal(result.Stdout, &document); err != nil {
		return Item{}, fmt.Errorf("op item get %s: %w", id, err)
	}
	item := Item{ID: document.ID, Title: document.Title, Vault: document.Vault.Name}
	for _, field := range document.Fields {
		item.Fields = append(item.Fields, Field{Label: field.Label, Reference: field.Reference, Type: field.Type})
	}
	return item, nil
}
