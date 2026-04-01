// Package gitprotocol handles Git smart HTTP protocol parsing.
package gitprotocol

// ServiceType identifies the git service being requested.
type ServiceType string

const (
	ServiceUploadPack  ServiceType = "git-upload-pack"
	ServiceReceivePack ServiceType = "git-receive-pack"
)

// RefUpdate represents a ref update command from git-receive-pack.
type RefUpdate struct {
	OldSHA  string
	NewSHA  string
	RefName string
}

// IsCreate returns true if this is a new ref creation.
func (r RefUpdate) IsCreate() bool {
	return r.OldSHA == "0000000000000000000000000000000000000000"
}

// IsDelete returns true if this is a ref deletion.
func (r RefUpdate) IsDelete() bool {
	return r.NewSHA == "0000000000000000000000000000000000000000"
}
