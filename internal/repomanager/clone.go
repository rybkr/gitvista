package repomanager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// sshShorthandRe matches SSH shorthand like git@github.com:user/repo.git.
var sshShorthandRe = regexp.MustCompile(`^([^@]+)@([^:]+):(.+)$`)

// normalizeURL canonicalizes a Git remote URL for deduplication.
// It lowercases the hostname, strips .git suffix and trailing slashes,
// removes embedded credentials, and converts SSH shorthand to ssh:// form.
func normalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("empty URL")
	}

	// Reject URLs that could be interpreted as git command-line options.
	if strings.HasPrefix(rawURL, "-") {
		return "", fmt.Errorf("invalid URL: must not start with '-'")
	}

	lower := strings.ToLower(rawURL)
	if strings.HasPrefix(lower, "file://") {
		return "", fmt.Errorf("file:// URLs are not supported")
	}
	if strings.HasPrefix(lower, "git://") {
		return "", fmt.Errorf("git:// URLs are not supported")
	}

	if m := sshShorthandRe.FindStringSubmatch(rawURL); m != nil {
		host := strings.ToLower(m[2])
		path := strings.TrimSuffix(m[3], ".git")
		path = strings.TrimRight(path, "/")
		return "ssh://" + host + "/" + path, nil
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" && scheme != "ssh" {
		return "", fmt.Errorf("unsupported scheme: %s", scheme)
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", fmt.Errorf("missing hostname")
	}

	if isPrivateHost(host) {
		return "", fmt.Errorf("cloning from private/internal addresses is not allowed")
	}

	port := parsed.Port()
	hostPart := host
	if port != "" {
		hostPart = host + ":" + port
	}

	path := parsed.Path
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimRight(path, "/")

	return scheme + "://" + hostPart + path, nil
}

// hashURL returns the first 16 characters of the SHA-256 hex digest of the
// normalized URL. The result is deterministic and filesystem-safe.
func hashURL(normalizedURL string) string {
	h := sha256.Sum256([]byte(normalizedURL))
	return fmt.Sprintf("%x", h)[:16]
}

// progressLineRe matches git progress lines like "Receiving objects:  45% (123/456)".
var progressLineRe = regexp.MustCompile(`^(.+?):\s+(\d+)%`)

// parseProgressLine extracts the phase and percent from a git progress line.
// Returns zero-value CloneProgress and false if the line doesn't match.
func parseProgressLine(line string) (CloneProgress, bool) {
	m := progressLineRe.FindStringSubmatch(line)
	if m == nil {
		return CloneProgress{}, false
	}
	pct, err := strconv.Atoi(m[2])
	if err != nil {
		return CloneProgress{}, false
	}
	return CloneProgress{Phase: m[1], Percent: pct}, true
}

// splitProgressLines splits a chunk of stderr output on \r and \n, returning
// individual progress lines. Git uses \r for in-place updates.
func splitProgressLines(chunk string) []string {
	var lines []string
	for _, part := range strings.Split(chunk, "\n") {
		for _, sub := range strings.Split(part, "\r") {
			sub = strings.TrimSpace(sub)
			if sub != "" {
				lines = append(lines, sub)
			}
		}
	}
	return lines
}

// cloneRepo performs a bare git clone of url into destPath.
// It uses the provided context for cancellation and enforces the given timeout.
// If onProgress is non-nil, it is called with parsed progress updates from git stderr.
// On failure, destPath is cleaned up.
func cloneRepo(ctx context.Context, repoURL, destPath string, timeout time.Duration, onProgress func(CloneProgress)) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//nolint:gosec // G204: URL is validated by normalizeURL before reaching here
	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", "--progress", "--", repoURL, destPath)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("clone stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("clone start: %w", err)
	}

	// Read stderr in a goroutine to avoid blocking the process.
	// Git progress uses \r for in-place updates within a single line,
	// so we read raw bytes and split on both \r and \n to get real-time updates.
	var stderrBuf strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		var partial strings.Builder
		for {
			n, readErr := stderr.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				stderrBuf.WriteString(chunk)
				// Accumulate into partial buffer and split on \r or \n.
				partial.WriteString(chunk)
				raw := partial.String()
				// Find the last \r or \n to determine what's complete.
				lastSep := -1
				for i := len(raw) - 1; i >= 0; i-- {
					if raw[i] == '\r' || raw[i] == '\n' {
						lastSep = i
						break
					}
				}
				if lastSep >= 0 {
					complete := raw[:lastSep+1]
					partial.Reset()
					partial.WriteString(raw[lastSep+1:])
					if onProgress != nil {
						for _, sub := range splitProgressLines(complete) {
							if p, ok := parseProgressLine(sub); ok {
								onProgress(p)
							}
						}
					}
				}
			}
			if readErr != nil {
				// Process any remaining partial line.
				if partial.Len() > 0 && onProgress != nil {
					if p, ok := parseProgressLine(strings.TrimSpace(partial.String())); ok {
						onProgress(p)
					}
				}
				break
			}
		}
	}()

	// Also drain any remaining data to prevent pipe blocking.
	// bufio.Scanner already reads from stderr, but the pipe close
	// signals EOF. We just need to wait for the goroutine.
	<-done

	if waitErr := cmd.Wait(); waitErr != nil {
		_ = os.RemoveAll(destPath)
		// Discard stderr data after draining.
		_, _ = io.Copy(io.Discard, stderr)

		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("clone timed out after %s", timeout)
		}
		return fmt.Errorf("clone failed: %s: %w", strings.TrimSpace(stderrBuf.String()), waitErr)
	}
	return nil
}

// fetchRepo runs git fetch --prune in the given bare repository path.
// It uses the provided context for cancellation and enforces the given timeout.
func fetchRepo(ctx context.Context, repoPath string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//nolint:gosec // G204: repoPath is a server-controlled bare repo directory, not user input
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fetch", "--prune", "--quiet")
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("fetch timed out after %s", timeout)
		}
		return fmt.Errorf("fetch failed: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}

// isPrivateHost returns true if the hostname resolves to a private, loopback,
// or link-local IP address. This prevents SSRF attacks where a user-supplied
// clone URL targets internal infrastructure (e.g., cloud metadata endpoints).
func isPrivateHost(host string) bool {
	switch host {
	case "localhost", "metadata.google.internal":
		return true
	}

	ips, err := net.DefaultResolver.LookupHost(context.Background(), host)
	if err != nil {
		ip := net.ParseIP(host)
		if ip == nil {
			return false
		}
		return isPrivateIP(ip)
	}

	for _, ipStr := range ips {
		ip := net.ParseIP(ipStr)
		if ip != nil && isPrivateIP(ip) {
			return true
		}
	}
	return false
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}
