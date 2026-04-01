package gitprotocol

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// ParseReceivePackCommands parses the command portion of a git-receive-pack request body.
// Returns the ref updates and a reader for the full body (for forwarding upstream).
func ParseReceivePackCommands(body io.Reader) ([]RefUpdate, io.Reader, error) {
	buffered := NewBufferedPktLineReader(body)
	var updates []RefUpdate

	for {
		pkt, err := buffered.ReadPacket()
		if err != nil {
			return nil, nil, fmt.Errorf("read pkt-line: %w", err)
		}

		// Flush packet marks end of commands
		if pkt == nil {
			break
		}

		// Parse the command line
		update, err := parseCommandLine(pkt)
		if err != nil {
			return nil, nil, fmt.Errorf("parse command: %w", err)
		}
		updates = append(updates, update)
	}

	// Reconstruct the full body: buffered commands (includes flush) + remainder (PACK data)
	fullBody := io.MultiReader(
		bytes.NewReader(buffered.Buffered()),
		buffered.Remainder(),
	)

	return updates, fullBody, nil
}

// parseCommandLine parses a single ref update command.
// Format: "<old-sha> <new-sha> <refname>[\0<capabilities>]"
func parseCommandLine(line []byte) (RefUpdate, error) {
	// Strip trailing newline if present
	line = bytes.TrimSuffix(line, []byte("\n"))

	// The first command line may have capabilities after a NUL byte
	if idx := bytes.IndexByte(line, 0); idx != -1 {
		line = line[:idx]
	}

	s := string(line)
	parts := strings.SplitN(s, " ", 3)
	if len(parts) != 3 {
		return RefUpdate{}, fmt.Errorf("invalid command format: %q", s)
	}

	oldSHA := parts[0]
	newSHA := parts[1]
	refName := parts[2]

	// Validate SHA format (40 hex chars)
	if len(oldSHA) != 40 || len(newSHA) != 40 {
		return RefUpdate{}, fmt.Errorf("invalid SHA length in command: %q", s)
	}

	return RefUpdate{
		OldSHA:  oldSHA,
		NewSHA:  newSHA,
		RefName: refName,
	}, nil
}

// WriteReceivePackError writes a proper git-receive-pack error response.
func WriteReceivePackError(w io.Writer, msg string) error {
	pktw := NewPktLineWriter(w)

	// Write unpack status
	if err := pktw.WritePacket([]byte("unpack ok\n")); err != nil {
		return err
	}

	// Write error for each ref (simplified - just write a general error)
	errLine := fmt.Sprintf("ng refs/heads/error %s\n", msg)
	if err := pktw.WritePacket([]byte(errLine)); err != nil {
		return err
	}

	return pktw.WriteFlush()
}
