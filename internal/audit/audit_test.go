package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 11, 23, 11, 30, 0, 0, time.FixedZone("CET", 3600))

var fixedCallers = []Process{{PID: 4242, Comm: "zsh", Argv: []string{"-zsh"}}}

func newRecorder(t *testing.T, audited ...string) *Recorder {
	t.Helper()
	return &Recorder{
		Path:    filepath.Join(t.TempDir(), "state", "audit.jsonl"),
		Via:     "get",
		Now:     func() time.Time { return fixedNow },
		Audited: func(entry string) bool { return slices.Contains(audited, entry) },
		Callers: func() []Process { return fixedCallers },
	}
}

func rawLines(t *testing.T, path string) [][]byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
}

func TestRecordOnNilRecorderDoesNothing(t *testing.T) {
	var recorder *Recorder
	if err := recorder.Record("github", ActionUse, ""); err != nil {
		t.Fatalf("nil recorder: %v", err)
	}
}

func TestRecordSkipsEntryThatIsNotAudited(t *testing.T) {
	recorder := newRecorder(t, "github")
	if err := recorder.Record("aws", ActionUse, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(recorder.Path); !os.IsNotExist(err) {
		t.Fatalf("audit log written for a non-audited entry: %v", err)
	}
}

func TestRecordWritesAuditOffForEntryThatIsNotAudited(t *testing.T) {
	recorder := newRecorder(t)
	if err := recorder.Record("aws", ActionAuditOff, "audit flag removed"); err != nil {
		t.Fatal(err)
	}
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Action != ActionAuditOff || events[0].Detail != "audit flag removed" {
		t.Fatalf("events = %+v", events)
	}
}

func recordOne(t *testing.T, recorder *Recorder) Event {
	t.Helper()
	if err := recorder.Record("github", ActionCopy, "field token"); err != nil {
		t.Fatal(err)
	}
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	return events[0]
}

func TestRecordCreatesLogWithMode0600(t *testing.T) {
	recorder := newRecorder(t, "github")
	recordOne(t, recorder)
	info, err := os.Stat(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestRecordStoresWhatAndWhenInUTC(t *testing.T) {
	got := recordOne(t, newRecorder(t, "github"))
	if !got.Time.Equal(fixedNow) || got.Time.Location() != time.UTC {
		t.Errorf("time = %v, want %v in UTC", got.Time, fixedNow)
	}
	if got.Entry != "github" || got.Action != ActionCopy || got.Via != "get" || got.Detail != "field token" || got.Prev != "" {
		t.Errorf("event = %+v", got)
	}
}

func TestRecordStoresWhoAndCallers(t *testing.T) {
	got := recordOne(t, newRecorder(t, "github"))
	current, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if got.User != current.Username || got.UID != os.Getuid() || got.Host != host || got.PID != os.Getpid() {
		t.Errorf("who = %+v", got)
	}
	if len(got.Callers) != 1 || got.Callers[0].Comm != "zsh" || !slices.Equal(got.Callers[0].Argv, []string{"-zsh"}) {
		t.Errorf("callers = %+v", got.Callers)
	}
}

func TestRecordChainsPrevToHashOfPreviousLine(t *testing.T) {
	recorder := newRecorder(t, "github")
	if err := recorder.Record("github", ActionUse, ""); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record("github", ActionView, ""); err != nil {
		t.Fatal(err)
	}
	lines := rawLines(t, recorder.Path)
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(lines[0])
	if len(events) != 2 || events[1].Prev != hex.EncodeToString(sum[:]) {
		t.Fatalf("second prev = %+v, want %x", events, sum)
	}
}

func TestConcurrentRecordsKeepChainIntact(t *testing.T) {
	recorder := newRecorder(t, "github")
	var group sync.WaitGroup
	errs := make(chan error, 20)
	for range 20 {
		group.Go(func() { errs <- recorder.Record("github", ActionUse, "") })
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	count, err := Verify(recorder.Path)
	if err != nil || count != 20 {
		t.Fatalf("Verify = %d, %v; want 20, nil", count, err)
	}
}

func TestVerifyNamesLineAfterEditedLine(t *testing.T) {
	recorder := newRecorder(t, "github")
	if err := recorder.Record("github", ActionUse, ""); err != nil {
		t.Fatal(err)
	}
	if err := recorder.Record("github", ActionView, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	edited := bytes.Replace(data, []byte(`"github"`), []byte(`"gitlab"`), 1)
	if err := os.WriteFile(recorder.Path, edited, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(recorder.Path)
	if err == nil || !strings.Contains(err.Error(), "line 2: chain broken") {
		t.Fatalf("Verify error = %v, want line 2: chain broken", err)
	}
}

func TestVerifyNamesUnparseableLine(t *testing.T) {
	recorder := newRecorder(t, "github")
	if err := recorder.Record("github", ActionUse, ""); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	data[0] = 'X'
	if err := os.WriteFile(recorder.Path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = Verify(recorder.Path)
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("Verify error = %v, want one naming line 1", err)
	}
}

func TestReadMissingFileReturnsNoEvents(t *testing.T) {
	events, err := Read(filepath.Join(t.TempDir(), "audit.jsonl"))
	if err != nil || events != nil {
		t.Fatalf("Read = %v, %v; want nil, nil", events, err)
	}
}

func TestCallerChainOfTestProcessIncludesNamedParent(t *testing.T) {
	chain := CallerChain(os.Getpid())
	if len(chain) < 2 || chain[0].PID != os.Getpid() || chain[1].PID != os.Getppid() || chain[1].Comm == "" {
		t.Fatalf("chain = %+v, want test %d then named parent %d", chain, os.Getpid(), os.Getppid())
	}
}

func TestSyncFlagsRecordsAuditOnForNewName(t *testing.T) {
	recorder := newRecorder(t)
	flags := filepath.Join(filepath.Dir(recorder.Path), "audit-flags.json")
	if err := SyncFlags(recorder, flags, []string{"github"}); err != nil {
		t.Fatal(err)
	}
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Entry != "github" || events[0].Action != ActionAuditOn {
		t.Fatalf("events = %+v, want one audit-on for github", events)
	}
}

func TestSyncFlagsRecordsNothingWhenUnchanged(t *testing.T) {
	recorder := newRecorder(t)
	flags := filepath.Join(filepath.Dir(recorder.Path), "audit-flags.json")
	if err := SyncFlags(recorder, flags, []string{"github"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncFlags(recorder, flags, []string{"github"}); err != nil {
		t.Fatal(err)
	}
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events after an unchanged sync, want 1", len(events))
	}
}

func TestSyncFlagsRecordsAuditOffForRemovedName(t *testing.T) {
	recorder := newRecorder(t)
	flags := filepath.Join(filepath.Dir(recorder.Path), "audit-flags.json")
	if err := SyncFlags(recorder, flags, []string{"github", "aws"}); err != nil {
		t.Fatal(err)
	}
	if err := SyncFlags(recorder, flags, []string{"aws"}); err != nil {
		t.Fatal(err)
	}
	events, err := Read(recorder.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want audit-on twice then audit-off", len(events))
	}
	last := events[2]
	if last.Entry != "github" || last.Action != ActionAuditOff || last.Detail != "audit flag removed" {
		t.Fatalf("events = %+v, want audit-off for github last", events)
	}
}
