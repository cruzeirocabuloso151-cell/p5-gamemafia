package state

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store is the persistence boundary. We use modernc.org/sqlite (pure Go)
// to keep the build CGo-free — important for cross-compiling to release
// binaries down the line.
type Store struct {
	db   *sql.DB
	path string
}

func OpenStore(campaignDir string) (*Store, error) {
	path := filepath.Join(campaignDir, "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

const schema = `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS characters (
  id        TEXT PRIMARY KEY,
  payload   TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS world_flags (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  ts        TEXT NOT NULL,
  kind      TEXT NOT NULL,
  text      TEXT NOT NULL
);
`

func (s *Store) migrate() error {
	_, err := s.db.Exec(schema)
	return err
}

// Save flushes the entire GameState. Called after every scene resolves.
// One transaction so a crash mid-save doesn't leave a corrupt half-state.
func (s *Store) Save(gs *GameState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('campaign',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, gs.Campaign); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO meta(key,value) VALUES('scene_index',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprintf("%d", gs.SceneIndex)); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM characters`); err != nil {
		return err
	}
	for _, c := range gs.Characters {
		buf, err := json.Marshal(c)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(
			`INSERT INTO characters(id,payload,updated_at) VALUES(?,?,?)`,
			c.ID, string(buf), time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DELETE FROM world_flags`); err != nil {
		return err
	}
	for k, v := range gs.WorldFlags {
		if _, err := tx.Exec(`INSERT INTO world_flags(key,value) VALUES(?,?)`, k, v); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// AppendEventDurable persists a single event row (in addition to the
// in-memory log on GameState). Lets us reconstruct the full timeline
// later for the Journal feature.
func (s *Store) AppendEventDurable(ev Event) error {
	_, err := s.db.Exec(`INSERT INTO events(ts,kind,text) VALUES(?,?,?)`,
		ev.Timestamp.UTC().Format(time.RFC3339), ev.Kind, ev.Text)
	return err
}

// Load rebuilds a GameState from disk. If the campaign is fresh (no
// rows), returns an empty state and (false, nil).
func (s *Store) Load(campaign string) (*GameState, bool, error) {
	gs := NewGameState(campaign)

	var sceneIdxStr string
	row := s.db.QueryRow(`SELECT value FROM meta WHERE key='scene_index'`)
	switch err := row.Scan(&sceneIdxStr); err {
	case nil:
		fmt.Sscanf(sceneIdxStr, "%d", &gs.SceneIndex)
	case sql.ErrNoRows:
		return gs, false, nil
	default:
		return gs, false, err
	}

	rows, err := s.db.Query(`SELECT payload FROM characters`)
	if err != nil {
		return gs, false, err
	}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			rows.Close()
			return gs, false, err
		}
		c := &Character{}
		if err := json.Unmarshal([]byte(raw), c); err != nil {
			rows.Close()
			return gs, false, err
		}
		gs.Characters = append(gs.Characters, c)
	}
	rows.Close()

	frows, err := s.db.Query(`SELECT key,value FROM world_flags`)
	if err != nil {
		return gs, false, err
	}
	for frows.Next() {
		var k, v string
		if err := frows.Scan(&k, &v); err != nil {
			frows.Close()
			return gs, false, err
		}
		gs.WorldFlags[k] = v
	}
	frows.Close()

	erows, err := s.db.Query(`SELECT id,ts,kind,text FROM events ORDER BY id DESC LIMIT 50`)
	if err != nil {
		return gs, true, err
	}
	for erows.Next() {
		var ev Event
		var ts string
		if err := erows.Scan(&ev.ID, &ts, &ev.Kind, &ev.Text); err != nil {
			erows.Close()
			return gs, true, err
		}
		ev.Timestamp, _ = time.Parse(time.RFC3339, ts)
		gs.Events = append(gs.Events, ev)
	}
	erows.Close()
	// reverse so oldest-first
	for i, j := 0, len(gs.Events)-1; i < j; i, j = i+1, j-1 {
		gs.Events[i], gs.Events[j] = gs.Events[j], gs.Events[i]
	}

	return gs, true, nil
}
