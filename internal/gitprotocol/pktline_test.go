package gitprotocol

import (
	"bytes"
	"io"
	"testing"
)

func TestPktLineReader(t *testing.T) {
	tests := []struct {
		name    string
		input   []byte
		want    [][]byte // nil = flush packet
		wantErr bool
	}{
		{
			name:  "single packet",
			input: []byte("000ahello\n"), // 0x000a = 10 = 4 (header) + 6 (payload "hello\n")
			want:  [][]byte{[]byte("hello\n")},
		},
		{
			name:  "flush packet",
			input: []byte("0000"),
			want:  [][]byte{nil},
		},
		{
			name:  "multiple packets",
			input: []byte("0006ab0007cde0000"),
			want:  [][]byte{[]byte("ab"), []byte("cde"), nil},
		},
		{
			name:    "invalid hex length",
			input:   []byte("ZZZZ"),
			wantErr: true,
		},
		{
			name:    "truncated packet",
			input:   []byte("000b"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := NewPktLineReader(bytes.NewReader(tt.input))

			var got [][]byte
			for {
				pkt, err := reader.ReadPacket()
				if err == io.EOF {
					break
				}
				if err != nil {
					if !tt.wantErr {
						t.Errorf("ReadPacket() unexpected error: %v", err)
					}
					return
				}
				got = append(got, pkt)
			}

			if tt.wantErr {
				t.Error("ReadPacket() expected error, got none")
				return
			}

			if len(got) != len(tt.want) {
				t.Errorf("ReadPacket() got %d packets, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if !bytes.Equal(got[i], tt.want[i]) {
					t.Errorf("packet[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestPktLineWriter(t *testing.T) {
	tests := []struct {
		name  string
		write func(*PktLineWriter) error
		want  []byte
	}{
		{
			name: "write packet",
			write: func(w *PktLineWriter) error {
				return w.WritePacket([]byte("hello"))
			},
			want: []byte("0009hello"),
		},
		{
			name: "write flush",
			write: func(w *PktLineWriter) error {
				return w.WriteFlush()
			},
			want: []byte("0000"),
		},
		{
			name: "write error",
			write: func(w *PktLineWriter) error {
				return w.WriteError("something went wrong")
			},
			want: []byte("001dERR something went wrong\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			writer := NewPktLineWriter(&buf)

			if err := tt.write(writer); err != nil {
				t.Fatalf("write error: %v", err)
			}

			if !bytes.Equal(buf.Bytes(), tt.want) {
				t.Errorf("got %q, want %q", buf.Bytes(), tt.want)
			}
		})
	}
}

func TestEncodePktLine(t *testing.T) {
	got := EncodePktLine([]byte("test"))
	want := []byte("0008test")

	if !bytes.Equal(got, want) {
		t.Errorf("EncodePktLine() = %q, want %q", got, want)
	}
}

func TestBufferedPktLineReader(t *testing.T) {
	// Simulate a receive-pack body with commands + flush + pack data
	input := []byte("0006ab0007cde0000PACK...")

	reader := NewBufferedPktLineReader(bytes.NewReader(input))

	// Read commands
	pkt1, _ := reader.ReadPacket()
	pkt2, _ := reader.ReadPacket()
	flush, _ := reader.ReadPacket()

	if string(pkt1) != "ab" {
		t.Errorf("pkt1 = %q, want %q", pkt1, "ab")
	}
	if string(pkt2) != "cde" {
		t.Errorf("pkt2 = %q, want %q", pkt2, "cde")
	}
	if flush != nil {
		t.Error("expected flush packet (nil)")
	}

	// Check buffered content (includes flush packet "0000" that was read)
	buffered := reader.Buffered()
	wantBuffered := []byte("0006ab0007cde0000")
	if !bytes.Equal(buffered, wantBuffered) {
		t.Errorf("Buffered() = %q, want %q", buffered, wantBuffered)
	}

	// Check remainder
	remainder, _ := io.ReadAll(reader.Remainder())
	wantRemainder := []byte("PACK...")
	if !bytes.Equal(remainder, wantRemainder) {
		t.Errorf("Remainder() = %q, want %q", remainder, wantRemainder)
	}
}
