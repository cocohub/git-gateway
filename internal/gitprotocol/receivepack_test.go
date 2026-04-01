package gitprotocol

import (
	"bytes"
	"io"
	"testing"
)

func TestParseReceivePackCommands(t *testing.T) {
	// Build a realistic receive-pack body
	// Format: "<old-sha> <new-sha> <refname>\0<capabilities>\n"
	// followed by more command lines, then flush (0000), then PACK data

	oldSHA := "0000000000000000000000000000000000000000"
	newSHA := "1234567890abcdef1234567890abcdef12345678"
	refName := "refs/heads/feature/test"
	capabilities := "report-status delete-refs side-band-64k"

	// First command line with capabilities
	cmd1 := oldSHA + " " + newSHA + " " + refName + "\x00" + capabilities + "\n"
	cmd1Pkt := EncodePktLine([]byte(cmd1))

	// Second command (no capabilities)
	oldSHA2 := "abcdef1234567890abcdef1234567890abcdef12"
	newSHA2 := "fedcba0987654321fedcba0987654321fedcba09"
	refName2 := "refs/heads/agent/work"
	cmd2 := oldSHA2 + " " + newSHA2 + " " + refName2 + "\n"
	cmd2Pkt := EncodePktLine([]byte(cmd2))

	// Build full body
	var body bytes.Buffer
	body.Write(cmd1Pkt)
	body.Write(cmd2Pkt)
	body.Write(EncodeFlush())
	body.Write([]byte("PACK\x00\x00\x00\x02...")) // Fake pack data

	updates, fullBody, err := ParseReceivePackCommands(&body)
	if err != nil {
		t.Fatalf("ParseReceivePackCommands() error: %v", err)
	}

	// Verify parsed updates
	if len(updates) != 2 {
		t.Fatalf("got %d updates, want 2", len(updates))
	}

	if updates[0].OldSHA != oldSHA {
		t.Errorf("updates[0].OldSHA = %q, want %q", updates[0].OldSHA, oldSHA)
	}
	if updates[0].NewSHA != newSHA {
		t.Errorf("updates[0].NewSHA = %q, want %q", updates[0].NewSHA, newSHA)
	}
	if updates[0].RefName != refName {
		t.Errorf("updates[0].RefName = %q, want %q", updates[0].RefName, refName)
	}

	if updates[1].OldSHA != oldSHA2 {
		t.Errorf("updates[1].OldSHA = %q, want %q", updates[1].OldSHA, oldSHA2)
	}
	if updates[1].RefName != refName2 {
		t.Errorf("updates[1].RefName = %q, want %q", updates[1].RefName, refName2)
	}

	// Verify full body is reconstructed correctly
	reconstructed, _ := io.ReadAll(fullBody)

	// Should contain: cmd1Pkt + cmd2Pkt + flush + pack data
	expectedPrefix := append(cmd1Pkt, cmd2Pkt...)
	expectedPrefix = append(expectedPrefix, EncodeFlush()...)

	if !bytes.HasPrefix(reconstructed, expectedPrefix) {
		t.Errorf("reconstructed body doesn't start with expected prefix")
	}

	if !bytes.Contains(reconstructed, []byte("PACK")) {
		t.Error("reconstructed body doesn't contain PACK data")
	}
}

func TestParseCommandLine(t *testing.T) {
	tests := []struct {
		name    string
		line    []byte
		want    RefUpdate
		wantErr bool
	}{
		{
			name: "basic command",
			line: []byte("0000000000000000000000000000000000000000 1234567890abcdef1234567890abcdef12345678 refs/heads/main\n"),
			want: RefUpdate{
				OldSHA:  "0000000000000000000000000000000000000000",
				NewSHA:  "1234567890abcdef1234567890abcdef12345678",
				RefName: "refs/heads/main",
			},
		},
		{
			name: "command with capabilities",
			line: []byte("0000000000000000000000000000000000000000 1234567890abcdef1234567890abcdef12345678 refs/heads/main\x00report-status\n"),
			want: RefUpdate{
				OldSHA:  "0000000000000000000000000000000000000000",
				NewSHA:  "1234567890abcdef1234567890abcdef12345678",
				RefName: "refs/heads/main",
			},
		},
		{
			name:    "invalid format",
			line:    []byte("invalid"),
			wantErr: true,
		},
		{
			name:    "invalid SHA length",
			line:    []byte("abc def refs/heads/main"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommandLine(tt.line)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCommandLine() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseCommandLine() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRefUpdate_IsCreate(t *testing.T) {
	create := RefUpdate{OldSHA: "0000000000000000000000000000000000000000", NewSHA: "abc"}
	if !create.IsCreate() {
		t.Error("IsCreate() should return true for zero old SHA")
	}

	update := RefUpdate{OldSHA: "abc", NewSHA: "def"}
	if update.IsCreate() {
		t.Error("IsCreate() should return false for non-zero old SHA")
	}
}

func TestRefUpdate_IsDelete(t *testing.T) {
	del := RefUpdate{OldSHA: "abc", NewSHA: "0000000000000000000000000000000000000000"}
	if !del.IsDelete() {
		t.Error("IsDelete() should return true for zero new SHA")
	}

	update := RefUpdate{OldSHA: "abc", NewSHA: "def"}
	if update.IsDelete() {
		t.Error("IsDelete() should return false for non-zero new SHA")
	}
}
