package config

import (
	"database/sql"
	"fmt"
)

// migrateDedupeUpstreams removes exact duplicate upstreams created before the
// dedupe keys covered every import path. The oldest row wins; accounts and
// subscription mappings are repointed to it so nothing breaks.
func migrateDedupeUpstreams(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, created_at, type, host, username, password,
		method, uuid, sni, security, pbk, sid, flow, fingerprint, transport,
		path, host_header, service_name, vmess_security
		FROM upstreams ORDER BY created_at, id`)
	if err != nil {
		return err
	}
	type rec struct {
		id  string
		key string
	}
	var order []rec
	groups := map[string][]string{}
	for rows.Next() {
		var id, typ, host, user, pass, method, uuid, sni, security, pbk, sid, flow, fingerprint, transport, path, hostHeader, serviceName, vmessSecurity string
		var created int64
		if err := rows.Scan(&id, &created, &typ, &host, &user, &pass,
			&method, &uuid, &sni, &security, &pbk, &sid, &flow, &fingerprint,
			&transport, &path, &hostHeader, &serviceName, &vmessSecurity); err != nil {
			rows.Close()
			return err
		}
		// Keys are built from plaintext passwords; fall back to the stored form
		// when a row cannot be decrypted (it still dedupes identical bad rows).
		if plain, derr := openSecret(pass); derr == nil {
			pass = plain
		}
		u := &Upstream{
			ID: id, Type: typ, Host: host, Username: user, Password: pass,
			Method: method, UUID: uuid, SNI: sni, Security: security,
			PBK: pbk, SID: sid, Flow: flow, Fingerprint: fingerprint,
			Transport: transport, Path: path, HostHeader: hostHeader,
			ServiceName: serviceName, VMessSecurity: vmessSecurity,
		}
		key := upstreamDedupKey(u)
		order = append(order, rec{id: id, key: key})
		groups[key] = append(groups[key], id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, item := range order {
		ids := groups[item.key]
		if len(ids) < 2 || ids[0] == "" {
			continue
		}
		keeper := ids[0]
		for _, dup := range ids[1:] {
			if dup == keeper {
				continue
			}
			if _, err := tx.Exec(`UPDATE accounts SET upstream_id=? WHERE upstream_id=?`, keeper, dup); err != nil {
				return fmt.Errorf("重新绑定账户失败: %w", err)
			}
			if _, err := tx.Exec(`INSERT OR IGNORE INTO subscription_nodes
				(subscription_id, upstream_id, node_key)
				SELECT subscription_id, ?, node_key FROM subscription_nodes WHERE upstream_id=?`,
				keeper, dup); err != nil {
				return fmt.Errorf("合并订阅映射失败: %w", err)
			}
			if _, err := tx.Exec(`DELETE FROM subscription_nodes WHERE upstream_id=?`, dup); err != nil {
				return fmt.Errorf("清理订阅映射失败: %w", err)
			}
			if _, err := tx.Exec(`DELETE FROM upstreams WHERE id=?`, dup); err != nil {
				return fmt.Errorf("删除重复上游失败: %w", err)
			}
		}
		// Mark the group handled so later iterations do not repeat the work.
		delete(groups, item.key)
	}
	return nil
}
