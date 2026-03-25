package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rybkr/gitvista/gitcore"
	"github.com/rybkr/gitvista/internal/repositoryview"
)

var errRepoUnavailable = errors.New("repository not available")

func (rs *RepoSession) handleBroadcast() {
	defer rs.wg.Done()

	for {
		select {
		case <-rs.ctx.Done():
			rs.logger.Debug("Broadcast handler exiting")
			return
		case message := <-rs.broadcast:
			rs.sendToAllClients(message)
		}
	}
}

func packetCommitCount(message UpdateMessage) int {
	switch {
	case message.Delta != nil:
		return len(message.Delta.AddedCommits)
	case message.Bootstrap != nil:
		return len(message.Bootstrap.Commits)
	default:
		return 0
	}
}

func marshalPacketPayload(message UpdateMessage) ([]byte, int, error) {
	commitCount := packetCommitCount(message)
	payload, err := json.Marshal(message)
	if err != nil {
		return nil, commitCount, err
	}
	return payload, commitCount, nil
}

func packetType(message UpdateMessage) string {
	if message.Type != "" {
		return message.Type
	}
	switch {
	case message.Delta != nil:
		return messageTypeGraphDelta
	case message.Status != nil:
		return messageTypeStatus
	case message.Head != nil:
		return messageTypeHead
	default:
		return "unknown"
	}
}

func logPacketSent(logger *slog.Logger, kind string, clients int, commitCount int, payloadBytes int) {
	logger.Debug("Packet sent",
		"type", kind,
		"clients", clients,
		"commits", commitCount,
		"bytes", payloadBytes,
		"totalBytes", payloadBytes*clients,
	)
}

func (rs *RepoSession) sendToAllClients(message UpdateMessage) {
	var failedClients []*websocket.Conn
	payload, commitCount, err := marshalPacketPayload(message)
	if err != nil {
		rs.logger.Error("Failed to serialize outbound packet", "type", packetType(message), "err", err)
		return
	}
	sentClients := 0

	rs.clientsMu.RLock()
	snapshot := make(map[*websocket.Conn]*sync.Mutex, len(rs.clients))
	for conn, mu := range rs.clients {
		snapshot[conn] = mu
	}
	rs.clientsMu.RUnlock()

	for conn, mu := range snapshot {
		mu.Lock()
		err1 := conn.SetWriteDeadline(time.Now().Add(writeWait))
		var err2 error
		if err1 == nil {
			err2 = conn.WriteMessage(websocket.TextMessage, payload)
		}
		mu.Unlock()

		if err1 != nil {
			rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err1)
			failedClients = append(failedClients, conn)
		} else if err2 != nil {
			rs.logger.Error("Broadcast failed", "addr", conn.RemoteAddr(), "err", err2)
			failedClients = append(failedClients, conn)
		} else {
			sentClients++
		}
	}

	if sentClients > 0 {
		logPacketSent(rs.logger, packetType(message), sentClients, commitCount, len(payload))
	}

	if len(failedClients) > 0 {
		rs.clientsMu.Lock()
		for _, conn := range failedClients {
			delete(rs.clients, conn)
			if err := conn.Close(); err != nil {
				rs.logger.Error("Failed to close client connection", "err", err)
			}
		}
		remainingClients := len(rs.clients)
		rs.clientsMu.Unlock()

		rs.logger.Info("Removed failed clients",
			"removed", len(failedClients),
			"remaining", remainingClients,
		)
	}
}

func (rs *RepoSession) broadcastUpdate(message UpdateMessage) {
	select {
	case rs.broadcast <- message:
	default:
		rs.logger.Warn("Broadcast channel full, dropping message; clients may be slow")
	}
}

func (rs *RepoSession) sendMessage(conn *websocket.Conn, message UpdateMessage) error {
	payload, commitCount, err := marshalPacketPayload(message)
	if err != nil {
		return err
	}
	if err := conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
		return err
	}
	logPacketSent(rs.logger, packetType(message), 1, commitCount, len(payload))
	return nil
}

func (rs *RepoSession) sendInitialRepoSummary(conn *websocket.Conn, repoSummary repositoryResponse) error {
	if err := rs.sendMessage(conn, UpdateMessage{
		Type: messageTypeRepoSummary,
		Repo: &repoSummary,
	}); err != nil {
		return err
	}
	return nil
}

func (rs *RepoSession) sendInitialBootstrap(conn *websocket.Conn) error {
	repo := rs.Repo()
	if repo == nil {
		return errRepoUnavailable
	}

	delta := repositoryview.DiffRepositories(repo, gitcore.NewEmptyRepository())
	for _, msg := range buildBootstrapMessages(delta) {
		if err := rs.sendMessage(conn, msg); err != nil {
			return err
		}
	}
	status := getWorkingTreeStatus(repo)
	if status == nil {
		status = &WorkingTreeStatus{
			Staged:    []FileStatus{},
			Modified:  []FileStatus{},
			Untracked: []FileStatus{},
		}
	}
	if err := rs.sendMessage(conn, UpdateMessage{Type: messageTypeStatus, Status: status}); err != nil {
		return err
	}
	headInfo := buildHeadInfo(repo)
	if headInfo != nil {
		if err := rs.sendMessage(conn, UpdateMessage{Type: messageTypeHead, Head: headInfo}); err != nil {
			return err
		}
	}
	return nil
}

func (rs *RepoSession) registerClient(conn *websocket.Conn) *sync.Mutex {
	writeMu := &sync.Mutex{}

	rs.clientsMu.Lock()
	rs.clients[conn] = writeMu
	clientCount := len(rs.clients)
	rs.clientsMu.Unlock()

	rs.logger.Info("WebSocket client registered", "addr", conn.RemoteAddr(), "totalClients", clientCount)
	return writeMu
}

func (rs *RepoSession) removeClient(conn *websocket.Conn) {
	rs.clientsMu.Lock()
	defer rs.clientsMu.Unlock()

	if _, ok := rs.clients[conn]; ok {
		delete(rs.clients, conn)
		if err := conn.Close(); err != nil {
			rs.logger.Error("Failed to close connection", "addr", conn.RemoteAddr(), "err", err)
		}
		rs.logger.Info("WebSocket client removed", "totalClients", len(rs.clients))
	}
}

func (rs *RepoSession) clientReadPump(conn *websocket.Conn, done chan struct{}) {
	defer rs.clientWg.Done()
	defer func() {
		if r := recover(); r != nil {
			rs.logger.Warn("Recovered panic in clientReadPump", "addr", conn.RemoteAddr(), "panic", r)
		}
		close(done)
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				rs.logger.Error("WebSocket read error", "addr", conn.RemoteAddr(), "err", err)
			}
			return
		}
	}
}

func (rs *RepoSession) clientWritePump(conn *websocket.Conn, done chan struct{}, writeMu *sync.Mutex) {
	defer rs.clientWg.Done()
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	defer rs.removeClient(conn)

	for {
		select {
		case <-done:
			rs.logger.Info("WebSocket client disconnected", "addr", conn.RemoteAddr())
			return

		case <-ticker.C:
			writeMu.Lock()
			err1 := conn.SetWriteDeadline(time.Now().Add(writeWait))
			var err2 error
			if err1 == nil {
				err2 = conn.WriteMessage(websocket.PingMessage, nil)
			}
			writeMu.Unlock()

			if err1 != nil {
				rs.logger.Error("Failed to set write deadline", "addr", conn.RemoteAddr(), "err", err1)
			}
			if err2 != nil {
				rs.logger.Error("WebSocket ping failed", "addr", conn.RemoteAddr(), "err", err2)
				return
			}
		}
	}
}

// StartFetchTicker starts a periodic repository refresh loop for the session.
func (rs *RepoSession) StartFetchTicker(interval time.Duration) {
	rs.wg.Add(1)
	go func() {
		defer rs.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-rs.ctx.Done():
				return
			case <-ticker.C:
				rs.updateRepository()
			}
		}
	}()
}

func buildHeadInfo(repo *gitcore.Repository) *HeadInfo {
	headRef := repo.HeadRef()

	branchName := ""
	if headRef != "" {
		if name, ok := strings.CutPrefix(headRef, "refs/heads/"); ok {
			branchName = name
		}
	}

	tagNames := repo.TagNames()
	recentTags := tagNames
	if len(tagNames) > 5 {
		recentTags = tagNames[:5]
	}

	return &HeadInfo{
		Hash:        string(repo.Head()),
		Ref:         headRef,
		BranchName:  branchName,
		IsDetached:  repo.HeadDetached(),
		Upstream:    repo.CurrentBranchUpstream(),
		CommitCount: repo.CommitCount(),
		BranchCount: len(repo.Branches()),
		TagCount:    len(tagNames),
		Description: repo.Description(),
		Remotes:     repo.Remotes(),
		RecentTags:  recentTags,
	}
}
