package gitcore

// StashEntry represents a single Git stash entry with its hash and message.
type StashEntry struct {
	Hash    Hash   `json:"hash"`
	Message string `json:"message"`
}
