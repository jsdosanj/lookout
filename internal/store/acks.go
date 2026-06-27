package store

import "time"

// AckRow is one persisted acknowledgement: an operator silenced the reminder
// cascade for an open incident on (RuleID, Server) until Until. Persisting acks
// means a control-plane restart does not re-page an operator for a problem they
// already acknowledged. The alert engine consumes these via a thin adapter so the
// store stays decoupled from the alert package.
type AckRow struct {
	RuleID string
	Server string
	Until  time.Time
}

// SaveAck upserts an acknowledgement.
func (s *Store) SaveAck(ruleID, server string, until time.Time) error {
	_, err := s.db.Exec(
		`INSERT INTO acks (rule_id, server, until) VALUES (?, ?, ?)
		 ON CONFLICT(rule_id, server) DO UPDATE SET until=excluded.until`,
		ruleID, server, until.UnixNano(),
	)
	return err
}

// DeleteAck removes an acknowledgement (no error if absent).
func (s *Store) DeleteAck(ruleID, server string) error {
	_, err := s.db.Exec(`DELETE FROM acks WHERE rule_id = ? AND server = ?`, ruleID, server)
	return err
}

// Acks returns all persisted acknowledgements.
func (s *Store) Acks() ([]AckRow, error) {
	rows, err := s.db.Query(`SELECT rule_id, server, until FROM acks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AckRow
	for rows.Next() {
		var (
			ruleID, server string
			until          int64
		)
		if err := rows.Scan(&ruleID, &server, &until); err != nil {
			return nil, err
		}
		out = append(out, AckRow{RuleID: ruleID, Server: server, Until: time.Unix(0, until).UTC()})
	}
	return out, rows.Err()
}
