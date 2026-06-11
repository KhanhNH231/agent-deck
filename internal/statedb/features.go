package statedb

import (
	"database/sql"
	"fmt"
	"time"
)

// FeatureRow represents a feature row: a named unit of work whose worktrees
// live under the managed workspace root (wsw absorption, schema v11).
type FeatureRow struct {
	ID        string
	Name      string
	State     string // "active" | "parked"
	RootPath  string // <workspace-root>/<feature-slug>
	Conductor bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// FeatureRepoRow is one repo snapshot belonging to a feature. Captured at
// start time so later manifest edits never break parked features.
type FeatureRepoRow struct {
	FeatureID    string
	RepoName     string
	RepoPath     string
	Branch       string
	BaseRef      string
	WorktreePath string
}

// SaveFeature upserts a feature row and replaces its repo snapshot in one
// transaction.
func (s *StateDB) SaveFeature(f FeatureRow, repos []FeatureRepoRow) error {
	return withBusyRetry(func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()

		now := time.Now().Unix()
		created := f.CreatedAt.Unix()
		if f.CreatedAt.IsZero() {
			created = now
		}
		conductorInt := 0
		if f.Conductor {
			conductorInt = 1
		}
		if _, err := tx.Exec(`
			INSERT INTO features (id, name, state, root_path, conductor, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				name = excluded.name, state = excluded.state,
				root_path = excluded.root_path, conductor = excluded.conductor,
				updated_at = excluded.updated_at
		`, f.ID, f.Name, f.State, f.RootPath, conductorInt, created, now); err != nil {
			return fmt.Errorf("statedb: save feature: %w", err)
		}

		if _, err := tx.Exec(`DELETE FROM feature_repos WHERE feature_id = ?`, f.ID); err != nil {
			return fmt.Errorf("statedb: clear feature repos: %w", err)
		}
		for _, r := range repos {
			if _, err := tx.Exec(`
				INSERT INTO feature_repos (feature_id, repo_name, repo_path, branch, base_ref, worktree_path)
				VALUES (?, ?, ?, ?, ?, ?)
			`, f.ID, r.RepoName, r.RepoPath, r.Branch, r.BaseRef, r.WorktreePath); err != nil {
				return fmt.Errorf("statedb: save feature repo %s: %w", r.RepoName, err)
			}
		}
		return tx.Commit()
	})
}

func (s *StateDB) scanFeature(row *sql.Row) (FeatureRow, error) {
	var f FeatureRow
	var conductorInt int
	var createdUnix, updatedUnix int64
	err := row.Scan(&f.ID, &f.Name, &f.State, &f.RootPath, &conductorInt, &createdUnix, &updatedUnix)
	if err != nil {
		return FeatureRow{}, err
	}
	f.Conductor = conductorInt != 0
	f.CreatedAt = time.Unix(createdUnix, 0)
	f.UpdatedAt = time.Unix(updatedUnix, 0)
	return f, nil
}

const featureColumns = `id, name, state, root_path, conductor, created_at, updated_at`

// GetFeatureByName returns a feature and its repo snapshot by unique name.
func (s *StateDB) GetFeatureByName(name string) (FeatureRow, []FeatureRepoRow, error) {
	f, err := s.scanFeature(s.db.QueryRow(
		`SELECT `+featureColumns+` FROM features WHERE name = ?`, name))
	if err != nil {
		return FeatureRow{}, nil, fmt.Errorf("statedb: feature %q: %w", name, err)
	}
	repos, err := s.loadFeatureRepos(f.ID)
	return f, repos, err
}

// GetFeatureByID returns a feature and its repo snapshot by ID.
func (s *StateDB) GetFeatureByID(id string) (FeatureRow, []FeatureRepoRow, error) {
	f, err := s.scanFeature(s.db.QueryRow(
		`SELECT `+featureColumns+` FROM features WHERE id = ?`, id))
	if err != nil {
		return FeatureRow{}, nil, fmt.Errorf("statedb: feature id %q: %w", id, err)
	}
	repos, err := s.loadFeatureRepos(f.ID)
	return f, repos, err
}

func (s *StateDB) loadFeatureRepos(featureID string) ([]FeatureRepoRow, error) {
	rows, err := s.db.Query(`
		SELECT feature_id, repo_name, repo_path, branch, base_ref, worktree_path
		FROM feature_repos WHERE feature_id = ? ORDER BY repo_name
	`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []FeatureRepoRow
	for rows.Next() {
		var r FeatureRepoRow
		if err := rows.Scan(&r.FeatureID, &r.RepoName, &r.RepoPath, &r.Branch, &r.BaseRef, &r.WorktreePath); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListFeatures returns all features ordered by name.
func (s *StateDB) ListFeatures() ([]FeatureRow, error) {
	rows, err := s.db.Query(`SELECT ` + featureColumns + ` FROM features ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []FeatureRow
	for rows.Next() {
		var f FeatureRow
		var conductorInt int
		var createdUnix, updatedUnix int64
		if err := rows.Scan(&f.ID, &f.Name, &f.State, &f.RootPath, &conductorInt, &createdUnix, &updatedUnix); err != nil {
			return nil, err
		}
		f.Conductor = conductorInt != 0
		f.CreatedAt = time.Unix(createdUnix, 0)
		f.UpdatedAt = time.Unix(updatedUnix, 0)
		result = append(result, f)
	}
	return result, rows.Err()
}

// SetFeatureState updates a feature's lifecycle state ("active" | "parked").
func (s *StateDB) SetFeatureState(id, state string) error {
	return withBusyRetry(func() error {
		_, err := s.db.Exec(`UPDATE features SET state = ?, updated_at = ? WHERE id = ?`,
			state, time.Now().Unix(), id)
		return err
	})
}

// DeleteFeature removes a feature and its repo snapshot.
func (s *StateDB) DeleteFeature(id string) error {
	return withBusyRetry(func() error {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		if _, err := tx.Exec(`DELETE FROM feature_repos WHERE feature_id = ?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM features WHERE id = ?`, id); err != nil {
			return err
		}
		return tx.Commit()
	})
}
