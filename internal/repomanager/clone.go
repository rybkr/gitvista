package repomanager

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"regexp"
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

// cloneRepo performs a bare git clone of url into destPath.
// It uses the provided context for cancellation and enforces the given timeout.
// On failure, destPath is cleaned up.
func cloneRepo(ctx context.Context, repoURL, destPath string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	//nolint:gosec // G204: URL is validated by normalizeURL before reaching here
	cmd := exec.CommandContext(ctx, "git", "clone", "--bare", "--quiet", "--", repoURL, destPath)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")

	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(destPath)

		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("clone timed out after %s", timeout)
		}
		return fmt.Errorf("clone failed: %s: %w", strings.TrimSpace(string(output)), err)
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

	ips, err := net.LookupHost(host)
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
