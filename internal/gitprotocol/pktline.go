package gitprotocol

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
)

// PktLineReader reads pkt-line formatted data.
type PktLineReader struct {
	r io.Reader
}

// NewPktLineReader creates a pkt-line reader.
func NewPktLineReader(r io.Reader) *PktLineReader {
	return &PktLineReader{r: r}
}

// ReadPacket reads one pkt-line.
// Returns (payload, nil) for data lines, (nil, nil) for flush packets (0000).
// Returns (nil, io.EOF) at end of stream.
func (p *PktLineReader) ReadPacket() ([]byte, error) {
	var lenBuf [4]byte
	_, err := io.ReadFull(p.r, lenBuf[:])
	if err != nil {
		return nil, err
	}

	length, err := parseHexLength(lenBuf[:])
	if err != nil {
		return nil, fmt.Errorf("invalid pkt-line length %q: %w", string(lenBuf[:]), err)
	}

	// 0000 is the flush packet
	if length == 0 {
		return nil, nil
	}

	// Length includes the 4-byte header
	if length < 4 {
		return nil, fmt.Errorf("invalid pkt-line length: %d", length)
	}

	payloadLen := length - 4
	if payloadLen == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, payloadLen)
	_, err = io.ReadFull(p.r, payload)
	if err != nil {
		return nil, fmt.Errorf("read pkt-line payload: %w", err)
	}

	return payload, nil
}

func parseHexLength(b []byte) (int, error) {
	decoded := make([]byte, 2)
	_, err := hex.Decode(decoded, b)
	if err != nil {
		return 0, err
	}
	return int(decoded[0])<<8 | int(decoded[1]), nil
}

// PktLineWriter writes pkt-line formatted data.
type PktLineWriter struct {
	w io.Writer
}

// NewPktLineWriter creates a pkt-line writer.
func NewPktLineWriter(w io.Writer) *PktLineWriter {
	return &PktLineWriter{w: w}
}

// WritePacket writes a data packet.
func (p *PktLineWriter) WritePacket(data []byte) error {
	length := len(data) + 4
	if length > 65535 {
		return fmt.Errorf("pkt-line too long: %d bytes", len(data))
	}

	header := fmt.Sprintf("%04x", length)
	if _, err := p.w.Write([]byte(header)); err != nil {
		return err
	}
	_, err := p.w.Write(data)
	return err
}

// WriteFlush writes a flush packet (0000).
func (p *PktLineWriter) WriteFlush() error {
	_, err := p.w.Write([]byte("0000"))
	return err
}

// WriteError writes an error packet in git protocol format.
func (p *PktLineWriter) WriteError(msg string) error {
	return p.WritePacket([]byte("ERR " + msg + "\n"))
}

// EncodePktLine encodes data as a single pkt-line.
func EncodePktLine(data []byte) []byte {
	length := len(data) + 4
	header := fmt.Sprintf("%04x", length)
	return append([]byte(header), data...)
}

// EncodeFlush returns a flush packet.
func EncodeFlush() []byte {
	return []byte("0000")
}

// BufferedPktLineReader reads pkt-lines while buffering the raw bytes.
type BufferedPktLineReader struct {
	r   io.Reader
	buf bytes.Buffer
}

// NewBufferedPktLineReader creates a buffered pkt-line reader.
func NewBufferedPktLineReader(r io.Reader) *BufferedPktLineReader {
	return &BufferedPktLineReader{r: r}
}

// ReadPacket reads one pkt-line, buffering the raw bytes.
func (b *BufferedPktLineReader) ReadPacket() ([]byte, error) {
	var lenBuf [4]byte
	_, err := io.ReadFull(b.r, lenBuf[:])
	if err != nil {
		return nil, err
	}
	b.buf.Write(lenBuf[:])

	length, err := parseHexLength(lenBuf[:])
	if err != nil {
		return nil, fmt.Errorf("invalid pkt-line length: %w", err)
	}

	if length == 0 {
		return nil, nil // flush packet
	}

	if length < 4 {
		return nil, fmt.Errorf("invalid pkt-line length: %d", length)
	}

	payloadLen := length - 4
	if payloadLen == 0 {
		return []byte{}, nil
	}

	payload := make([]byte, payloadLen)
	_, err = io.ReadFull(b.r, payload)
	if err != nil {
		return nil, fmt.Errorf("read pkt-line payload: %w", err)
	}
	b.buf.Write(payload)

	return payload, nil
}

// Buffered returns all the raw bytes read so far.
func (b *BufferedPktLineReader) Buffered() []byte {
	return b.buf.Bytes()
}

// Remainder returns a reader for the rest of the underlying stream.
func (b *BufferedPktLineReader) Remainder() io.Reader {
	return b.r
}
