package config

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Subscription is a remote node list fetched periodically. The URL may carry a
// token, so it is sealed with the same AES-GCM key as passwords.
type Subscription struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	Enabled       bool   `json:"enabled"`
	IntervalSec   int    `json:"interval_sec"`
	LastFetchAt   int64  `json:"last_fetch_at"`
	LastSuccessAt int64  `json:"last_success_at"`
	LastError     string `json:"last_error,omitempty"`
	ETag          string `json:"-"`
	LastModified  string `json:"-"`
	CreatedAt     int64  `json:"created_at"`
	UpdatedAt     int64  `json:"updated_at"`
}

// SubscriptionInfo adds the number of nodes currently mapped to a subscription.
type SubscriptionInfo struct {
	Subscription
	NodeCount int `json:"node_count"`
}

// RefreshResult is the outcome of one subscription refresh.
type RefreshResult struct {
	Added    int      `json:"added"`
	Updated  int      `json:"updated"`
	Removed  int      `json:"removed"`
	Problems []string `json:"problems,omitempty"`
	Error    string   `json:"error,omitempty"`
}

const (
	defaultSubInterval = 3600
	minSubInterval     = 300
	maxSubInterval     = 86400
)

func migrateSubscriptions(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			interval_sec INTEGER NOT NULL DEFAULT 3600,
			last_fetch_at INTEGER NOT NULL DEFAULT 0,
			last_success_at INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			etag TEXT NOT NULL DEFAULT '',
			last_modified TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS subscription_nodes (
			subscription_id TEXT NOT NULL,
			upstream_id TEXT NOT NULL,
			node_key TEXT NOT NULL,
			PRIMARY KEY (subscription_id, upstream_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subscription_nodes_sub ON subscription_nodes(subscription_id)`,
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ListSubscriptions returns subscriptions with their mapped node count.
func (s *Store) ListSubscriptions() ([]SubscriptionInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT s.id, s.name, s.url, s.enabled, s.interval_sec,
		s.last_fetch_at, s.last_success_at, s.last_error, s.etag, s.last_modified,
		s.created_at, s.updated_at,
		(SELECT COUNT(*) FROM subscription_nodes n WHERE n.subscription_id=s.id)
		FROM subscriptions s ORDER BY s.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SubscriptionInfo
	for rows.Next() {
		var it SubscriptionInfo
		var rawURL string
		var enabled int
		if err := rows.Scan(&it.ID, &it.Name, &rawURL, &enabled, &it.IntervalSec,
			&it.LastFetchAt, &it.LastSuccessAt, &it.LastError, &it.ETag, &it.LastModified,
			&it.CreatedAt, &it.UpdatedAt, &it.NodeCount); err != nil {
			return nil, err
		}
		it.Enabled = enabled != 0
		it.URL = rawURL
		if err := openFields(&it.Name, &it.URL, &it.LastError, &it.ETag, &it.LastModified); err != nil {
			return nil, fmt.Errorf("订阅 %s 解密失败: %w", it.ID, err)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// GetSubscription loads one subscription (URL decrypted).
func (s *Store) GetSubscription(id string) (*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.getSubscriptionLocked(id)
}

func validateSubscription(in *Subscription) error {
	in.Name = strings.TrimSpace(in.Name)
	in.URL = strings.TrimSpace(in.URL)
	if in.Name == "" {
		return errors.New("订阅名称不能为空")
	}
	if in.URL == "" {
		return errors.New("订阅地址不能为空")
	}
	u, err := url.Parse(in.URL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("订阅地址必须是 http(s) URL")
	}
	if in.IntervalSec == 0 {
		in.IntervalSec = defaultSubInterval
	}
	if in.IntervalSec < minSubInterval || in.IntervalSec > maxSubInterval {
		return fmt.Errorf("刷新间隔需在 %d-%d 秒之间", minSubInterval, maxSubInterval)
	}
	return nil
}

// UpsertSubscription creates (id == "") or updates a subscription.
func (s *Store) UpsertSubscription(id string, in *Subscription) (*Subscription, error) {
	if err := validateSubscription(in); err != nil {
		return nil, err
	}
	if dup, err := s.subscriptionURLExists(in.URL, id); err != nil {
		return nil, err
	} else if dup {
		return nil, errors.New("该订阅地址已存在，请直接刷新现有订阅，避免重复节点")
	}
	sealedName, err := sealSecret(in.Name)
	if err != nil {
		return nil, err
	}
	sealed, err := sealSecret(in.URL)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" {
		in.ID = randomHex(4)
		_, err = s.db.Exec(`INSERT INTO subscriptions
			(id, name, url, enabled, interval_sec, created_at, updated_at)
			VALUES(?,?,?,?,?,?,?)`,
			in.ID, sealedName, sealed, boolInt(in.Enabled), in.IntervalSec, now, now)
	} else {
		res, uerr := s.db.Exec(`UPDATE subscriptions SET name=?, url=?, enabled=?,
			interval_sec=?, updated_at=? WHERE id=?`,
			sealedName, sealed, boolInt(in.Enabled), in.IntervalSec, now, id)
		if uerr != nil {
			return nil, uerr
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return nil, ErrNotFound
		}
		in.ID = id
	}
	if err != nil {
		return nil, err
	}
	return s.getSubscriptionLocked(in.ID)
}

// subscriptionURLExists reports whether another subscription already uses the
// same URL (the URL may contain a token, so compare decrypted plaintext).
func (s *Store) subscriptionURLExists(raw, excludeID string) (bool, error) {
	subs, err := s.ListSubscriptions()
	if err != nil {
		return false, err
	}
	for _, sub := range subs {
		if sub.ID != excludeID && sub.URL == raw {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) getSubscriptionLocked(id string) (*Subscription, error) {
	var it Subscription
	var rawURL string
	var enabled int
	err := s.db.QueryRow(`SELECT id, name, url, enabled, interval_sec, last_fetch_at,
		last_success_at, last_error, etag, last_modified, created_at, updated_at
		FROM subscriptions WHERE id=?`, id).
		Scan(&it.ID, &it.Name, &rawURL, &enabled, &it.IntervalSec, &it.LastFetchAt,
			&it.LastSuccessAt, &it.LastError, &it.ETag, &it.LastModified,
			&it.CreatedAt, &it.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	it.Enabled = enabled != 0
	it.URL = rawURL
	if err := openFields(&it.Name, &it.URL, &it.LastError, &it.ETag, &it.LastModified); err != nil {
		return nil, fmt.Errorf("订阅解密失败: %w", err)
	}
	return &it, nil
}

// DeleteSubscription removes a subscription. When deleteNodes is true it also
// removes its upstreams that are not referenced by any account.
func (s *Store) DeleteSubscription(id string, deleteNodes bool) error {
	sub, err := s.GetSubscription(id)
	if err != nil {
		return err
	}
	_ = sub
	nodes, err := s.SubscriptionNodeIDs(id)
	if err != nil {
		return err
	}
	if deleteNodes {
		for _, uid := range nodes {
			// Referenced nodes stay (the account would break); the mapping is
			// removed so they become normal manual upstreams.
			_ = s.DeleteUpstream(uid)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM subscription_nodes WHERE subscription_id=?`, id); err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM subscriptions WHERE id=?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SubscriptionNodeIDs lists the upstream ids mapped to a subscription.
func (s *Store) SubscriptionNodeIDs(id string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT upstream_id FROM subscription_nodes WHERE subscription_id=? ORDER BY upstream_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// RecordSubscriptionFetch stores the fetch outcome (etag/last-modified are kept
// on 304 or on failure so the next request can still be conditional).
func (s *Store) RecordSubscriptionFetch(id, etag, lastModified, fetchErr string, success bool) error {
	if err := sealFields(&etag, &lastModified, &fetchErr); err != nil {
		return err
	}
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	if success {
		_, err := s.db.Exec(`UPDATE subscriptions SET last_fetch_at=?, last_success_at=?,
			last_error='', etag=?, last_modified=?, updated_at=? WHERE id=?`,
			now, now, etag, lastModified, now, id)
		return err
	}
	_, err := s.db.Exec(`UPDATE subscriptions SET last_fetch_at=?, last_error=?,
		etag=?, last_modified=?, updated_at=? WHERE id=?`,
		now, fetchErr, etag, lastModified, now, id)
	return err
}

// ReplaceSubscriptionNodes reconciles the upstream pool with the parsed nodes.
// Existing mappings are updated in place; nodes no longer present are removed
// (unless an account references them, in which case the upstream survives as a
// normal node). Manual upstreams are never touched.
func (s *Store) ReplaceSubscriptionNodes(subID string, candidates []*Upstream) (RefreshResult, error) {
	var res RefreshResult
	if _, err := s.GetSubscription(subID); err != nil {
		return res, err
	}

	type mapped struct {
		upstreamID string
		key        string
	}
	existing := map[string]mapped{}
	s.mu.RLock()
	rows, err := s.db.Query(`SELECT upstream_id, node_key FROM subscription_nodes WHERE subscription_id=?`, subID)
	if err != nil {
		s.mu.RUnlock()
		return res, err
	}
	for rows.Next() {
		var m mapped
		if err := rows.Scan(&m.upstreamID, &m.key); err != nil {
			rows.Close()
			s.mu.RUnlock()
			return res, err
		}
		if plain, derr := openSecret(m.key); derr == nil {
			m.key = plain
		}
		existing[m.key] = m
	}
	rows.Close()
	s.mu.RUnlock()

	// Keys of every existing upstream (manual or another subscription). A
	// subscription refresh must not insert a second row for a node that is
	// already in the pool.
	allByKey := map[string]string{}
	for _, u := range s.Upstreams() {
		allByKey[upstreamDedupKey(u)] = u.ID
	}

	keep := map[string]bool{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if err := NormalizeUpstream(candidate); err != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s): %v", candidate.Name, candidate.Host, err))
			continue
		}
		key := upstreamDedupKey(candidate)
		if seen[key] {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s): 订阅内重复节点", candidate.Name, candidate.Host))
			continue
		}
		seen[key] = true

		if m, ok := existing[key]; ok {
			candidate.ID = m.upstreamID
			if _, err := s.UpsertUpstream(m.upstreamID, candidate); err == nil {
				keep[m.upstreamID] = true
				res.Updated++
				continue
			} else if !errors.Is(err, ErrNotFound) {
				res.Problems = append(res.Problems, fmt.Sprintf("%s (%s): %v", candidate.Name, candidate.Host, err))
				continue
			}
			// Mapped upstream was deleted earlier; fall through to recreate it.
		}

		// Already present as a manual upstream or another subscription's node:
		// recognize it but do not create a duplicate or take ownership (a later
		// subscription removal must not delete a manually added node).
		if _, ok := allByKey[key]; ok {
			res.Updated++
			continue
		}

		saved, err := s.UpsertUpstream("", candidate)
		if err != nil {
			res.Problems = append(res.Problems, fmt.Sprintf("%s (%s): %v", candidate.Name, candidate.Host, err))
			continue
		}
		sealedKey, err := sealSecret(key)
		if err != nil {
			return res, err
		}
		s.mu.Lock()
		_, err = s.db.Exec(`INSERT OR REPLACE INTO subscription_nodes
			(subscription_id, upstream_id, node_key) VALUES(?,?,?)`, subID, saved.ID, sealedKey)
		s.mu.Unlock()
		if err != nil {
			return res, err
		}
		keep[saved.ID] = true
		res.Added++
	}

	for key, m := range existing {
		if keep[m.upstreamID] || seen[key] {
			continue
		}
		s.mu.Lock()
		_, _ = s.db.Exec(`DELETE FROM subscription_nodes WHERE subscription_id=? AND upstream_id=?`, subID, m.upstreamID)
		s.mu.Unlock()
		if err := s.DeleteUpstream(m.upstreamID); err == nil {
			res.Removed++
		}
	}
	return res, nil
}
