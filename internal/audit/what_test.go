package audit

import "testing"

var terraformChain = []Process{
	{PID: 300, Comm: "terraform", Argv: []string{"/opt/homebrew/bin/terraform", "plan"}},
	{PID: 200, Comm: "zsh", Argv: []string{"-zsh"}},
	{PID: 199, Comm: "zsh", Argv: []string{"-zsh"}},
	{PID: 100, Comm: "login", Argv: nil},
}

func TestWhatRendersChainCollapsingRepeatedCallers(t *testing.T) {
	event := Event{Action: ActionUse, Callers: terraformChain}

	if got, want := event.What(), "terraform plan ← -zsh ← login"; got != want {
		t.Fatalf("What() = %q, want %q", got, want)
	}
}

func TestWhatPrefixesChainWithRunChildCommand(t *testing.T) {
	event := Event{Action: ActionUse, Detail: "sh", Callers: terraformChain[1:2]}

	if got, want := event.What(), "sh ← -zsh"; got != want {
		t.Fatalf("What() = %q, want %q", got, want)
	}
}

func TestWhatOfWriteReturnsDetailOnly(t *testing.T) {
	event := Event{Action: ActionWrite, Detail: "changed · commit 1a2b3c4", Callers: terraformChain}

	if got, want := event.What(), "changed · commit 1a2b3c4"; got != want {
		t.Fatalf("What() = %q, want %q", got, want)
	}
}

func TestWhatOfWriteWithoutDetailFallsBackToChain(t *testing.T) {
	event := Event{Action: ActionAuditOn, Callers: terraformChain[1:2]}

	if got, want := event.What(), "-zsh"; got != want {
		t.Fatalf("What() = %q, want %q", got, want)
	}
}
