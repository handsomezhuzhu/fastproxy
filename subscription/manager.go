package subscription

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/handsomezhuzhu/fastproxy/config"
	"github.com/handsomezhuzhu/fastproxy/importer"
)

// Logger is the subset of logx.Logger the manager needs.
type Logger interface {
	Infof(format string, args ...any)
	Warnf(format string, args ...any)
	Errorf(format string, args ...any)
}

// Manager refreshes subscriptions on a timer and reconciles their nodes.
type Manager struct {
	store    *config.Store
	log      Logger
	onChange func() // rebuild sing-box / sync listeners after node changes

	mu       sync.Mutex
	inflight map[string]bool
}

// NewManager creates a subscription manager. onChange may be nil.
func NewManager(store *config.Store, logger Logger, onChange func()) *Manager {
	return &Manager{store: store, log: logger, onChange: onChange, inflight: map[string]bool{}}
}

// Start periodically refreshes due subscriptions until ctx is cancelled.
func (m *Manager) Start(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	m.refreshDue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.refreshDue(ctx)
		}
	}
}

func (m *Manager) refreshDue(ctx context.Context) {
	subs, err := m.store.ListSubscriptions()
	if err != nil {
		if m.log != nil {
			m.log.Warnf("读取订阅列表失败: %v", err)
		}
		return
	}
	now := time.Now().Unix()
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, sub := range subs {
		if !sub.Enabled {
			continue
		}
		if sub.LastFetchAt > 0 && now < sub.LastFetchAt+int64(sub.IntervalSec) {
			continue
		}
		sub := sub
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if _, err := m.Refresh(ctx, sub.ID); err != nil && m.log != nil {
				m.log.Warnf("订阅 %s 刷新失败: %v", sub.Name, err)
			}
		}()
	}
	wg.Wait()
}

// Refresh fetches one subscription and applies its nodes. Concurrent refreshes
// of the same subscription are coalesced (the second call returns an error).
func (m *Manager) Refresh(ctx context.Context, id string) (config.RefreshResult, error) {
	var result config.RefreshResult

	m.mu.Lock()
	if m.inflight[id] {
		m.mu.Unlock()
		return result, errInFlight
	}
	m.inflight[id] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.inflight, id)
		m.mu.Unlock()
	}()

	sub, err := m.store.GetSubscription(id)
	if err != nil {
		return result, err
	}
	fetched, err := Fetch(ctx, sub.URL, sub.ETag, sub.LastModified)
	if err != nil {
		_ = m.store.RecordSubscriptionFetch(id, sub.ETag, sub.LastModified, err.Error(), false)
		result.Error = err.Error()
		return result, err
	}
	if fetched.NotModified {
		_ = m.store.RecordSubscriptionFetch(id, sub.ETag, sub.LastModified, "", true)
		return result, nil
	}
	parsed := importer.Parse(fetched.Body)
	result.Problems = append(result.Problems, parsed.Problems...)
	if len(parsed.Proxies) == 0 {
		msg := "订阅未解析到任何节点"
		if len(parsed.Problems) > 0 {
			msg += ": " + strings.Join(parsed.Problems, "; ")
		}
		_ = m.store.RecordSubscriptionFetch(id, fetched.ETag, fetched.LastModified, msg, false)
		result.Error = msg
		return result, errNoNodes
	}
	changed, err := m.store.ReplaceSubscriptionNodes(id, proxiesToUpstreams(parsed.Proxies))
	result.Added, result.Updated, result.Removed = changed.Added, changed.Updated, changed.Removed
	result.Problems = append(result.Problems, changed.Problems...)
	if err != nil {
		_ = m.store.RecordSubscriptionFetch(id, fetched.ETag, fetched.LastModified, err.Error(), false)
		result.Error = err.Error()
		return result, err
	}
	_ = m.store.RecordSubscriptionFetch(id, fetched.ETag, fetched.LastModified, "", true)
	if m.log != nil {
		m.log.Infof("订阅 %s 刷新完成: 新增 %d, 更新 %d, 移除 %d, 问题 %d",
			sub.Name, result.Added, result.Updated, result.Removed, len(result.Problems))
	}
	if (result.Added > 0 || result.Updated > 0 || result.Removed > 0) && m.onChange != nil {
		m.onChange()
	}
	return result, nil
}

var (
	errInFlight = &subError{"该订阅正在刷新，请稍后再试"}
	errNoNodes  = &subError{"订阅未解析到任何节点"}
)

type subError struct{ msg string }

func (e *subError) Error() string { return e.msg }

func proxiesToUpstreams(proxies []importer.Proxy) []*config.Upstream {
	out := make([]*config.Upstream, 0, len(proxies))
	for _, p := range proxies {
		out = append(out, &config.Upstream{
			Name: p.Name, Type: p.Type, Host: p.Host,
			Username: p.User, Password: p.Pass, Method: p.Method,
			UUID: p.UUID, SNI: p.SNI,
			Security: p.Security, PBK: p.PBK, SID: p.SID, Flow: p.Flow,
			Fingerprint: p.Fingerprint, Transport: p.Transport, Path: p.Path,
			HostHeader: p.HostHeader, ServiceName: p.ServiceName, VMessSecurity: p.VMessSecurity,
		})
	}
	return out
}
