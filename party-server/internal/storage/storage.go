package storage

import (
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Storage handles persistent data storage
type Storage struct {
	db *sql.DB
}

// NewStorage creates a new storage instance
func NewStorage(dataDir string) (*Storage, error) {
	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataDir, "party.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	s := &Storage{db: db}
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

// initSchema creates the database tables
func (s *Storage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS clients (
		client_id TEXT PRIMARY KEY,
		display_name TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_seen DATETIME DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS friends (
		client_id TEXT NOT NULL,
		friend_id TEXT NOT NULL,
		friend_name TEXT NOT NULL,
		added_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_played_with DATETIME,
		PRIMARY KEY (client_id, friend_id)
	);

	CREATE TABLE IF NOT EXISTS friend_requests (
		from_client_id TEXT NOT NULL,
		to_client_id TEXT NOT NULL,
		from_display_name TEXT NOT NULL,
		to_display_name TEXT NOT NULL,
		sent_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (from_client_id, to_client_id)
	);

	CREATE INDEX IF NOT EXISTS idx_friends_client ON friends(client_id);
	CREATE INDEX IF NOT EXISTS idx_friends_friend ON friends(friend_id);
	CREATE INDEX IF NOT EXISTS idx_friend_requests_from ON friend_requests(from_client_id);
	CREATE INDEX IF NOT EXISTS idx_friend_requests_to ON friend_requests(to_client_id);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Close closes the database connection
func (s *Storage) Close() error {
	return s.db.Close()
}

// RegisterClient registers or updates a client
func (s *Storage) RegisterClient(clientID, displayName string) error {
	_, err := s.db.Exec(`
		INSERT INTO clients (client_id, display_name, last_seen)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(client_id) DO UPDATE SET
			display_name = excluded.display_name,
			last_seen = CURRENT_TIMESTAMP
	`, clientID, displayName)
	return err
}

// GetClient retrieves a client by ID
func (s *Storage) GetClient(clientID string) (displayName string, exists bool, err error) {
	err = s.db.QueryRow(`SELECT display_name FROM clients WHERE client_id = ?`, clientID).Scan(&displayName)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return displayName, true, nil
}

// UpdateLastSeen updates the last seen timestamp for a client
func (s *Storage) UpdateLastSeen(clientID string) error {
	_, err := s.db.Exec(`UPDATE clients SET last_seen = CURRENT_TIMESTAMP WHERE client_id = ?`, clientID)
	return err
}

// AddFriend adds a friend relationship (bidirectional)
func (s *Storage) AddFriend(clientID, friendID, friendName string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Get client's display name
	var clientName string
	err = tx.QueryRow(`SELECT display_name FROM clients WHERE client_id = ?`, clientID).Scan(&clientName)
	if err != nil {
		return err
	}

	// Add both directions
	_, err = tx.Exec(`
		INSERT INTO friends (client_id, friend_id, friend_name)
		VALUES (?, ?, ?)
		ON CONFLICT(client_id, friend_id) DO UPDATE SET friend_name = excluded.friend_name
	`, clientID, friendID, friendName)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO friends (client_id, friend_id, friend_name)
		VALUES (?, ?, ?)
		ON CONFLICT(client_id, friend_id) DO UPDATE SET friend_name = excluded.friend_name
	`, friendID, clientID, clientName)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// RemoveFriend removes a friend relationship (bidirectional)
func (s *Storage) RemoveFriend(clientID, friendID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM friends WHERE client_id = ? AND friend_id = ?`, clientID, friendID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM friends WHERE client_id = ? AND friend_id = ?`, friendID, clientID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// GetFriends retrieves all friends for a client
func (s *Storage) GetFriends(clientID string) ([]FriendRecord, error) {
	rows, err := s.db.Query(`
		SELECT friend_id, friend_name, added_at, last_played_with
		FROM friends
		WHERE client_id = ?
		ORDER BY last_played_with DESC NULLS LAST, added_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []FriendRecord
	for rows.Next() {
		var f FriendRecord
		var lastPlayed sql.NullTime
		if err := rows.Scan(&f.FriendID, &f.FriendName, &f.AddedAt, &lastPlayed); err != nil {
			return nil, err
		}
		if lastPlayed.Valid {
			f.LastPlayedWith = &lastPlayed.Time
		}
		friends = append(friends, f)
	}

	return friends, rows.Err()
}

// AreFriends checks if two clients are friends
func (s *Storage) AreFriends(clientID, friendID string) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM friends WHERE client_id = ? AND friend_id = ?
	`, clientID, friendID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// UpdateLastPlayedWith updates the last played timestamp for friends
func (s *Storage) UpdateLastPlayedWith(clientID, friendID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now()
	_, err = tx.Exec(`
		UPDATE friends SET last_played_with = ? WHERE client_id = ? AND friend_id = ?
	`, now, clientID, friendID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE friends SET last_played_with = ? WHERE client_id = ? AND friend_id = ?
	`, now, friendID, clientID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// FriendRecord represents a friend from storage
type FriendRecord struct {
	FriendID       string
	FriendName     string
	AddedAt        time.Time
	LastPlayedWith *time.Time
}

// FriendRequestRecord represents a friend request from storage
type FriendRequestRecord struct {
	ClientID    string
	DisplayName string
	SentAt      time.Time
}

// CreateFriendRequest creates a new friend request
func (s *Storage) CreateFriendRequest(fromClientID, toClientID, fromDisplayName, toDisplayName string) error {
	_, err := s.db.Exec(`
		INSERT INTO friend_requests (from_client_id, to_client_id, from_display_name, to_display_name)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(from_client_id, to_client_id) DO NOTHING
	`, fromClientID, toClientID, fromDisplayName, toDisplayName)
	return err
}

// DeleteFriendRequest deletes a friend request
func (s *Storage) DeleteFriendRequest(fromClientID, toClientID string) error {
	_, err := s.db.Exec(`DELETE FROM friend_requests WHERE from_client_id = ? AND to_client_id = ?`, fromClientID, toClientID)
	return err
}

// GetFriendRequest checks if a friend request exists
func (s *Storage) GetFriendRequest(fromClientID, toClientID string) (exists bool, err error) {
	var count int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM friend_requests WHERE from_client_id = ? AND to_client_id = ?
	`, fromClientID, toClientID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetIncomingFriendRequests gets all friend requests sent TO a client
func (s *Storage) GetIncomingFriendRequests(clientID string) ([]FriendRequestRecord, error) {
	rows, err := s.db.Query(`
		SELECT from_client_id, from_display_name, sent_at
		FROM friend_requests
		WHERE to_client_id = ?
		ORDER BY sent_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []FriendRequestRecord
	for rows.Next() {
		var r FriendRequestRecord
		if err := rows.Scan(&r.ClientID, &r.DisplayName, &r.SentAt); err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}

	return requests, rows.Err()
}

// GetOutgoingFriendRequests gets all friend requests sent BY a client
func (s *Storage) GetOutgoingFriendRequests(clientID string) ([]FriendRequestRecord, error) {
	rows, err := s.db.Query(`
		SELECT to_client_id, to_display_name, sent_at
		FROM friend_requests
		WHERE from_client_id = ?
		ORDER BY sent_at DESC
	`, clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var requests []FriendRequestRecord
	for rows.Next() {
		var r FriendRequestRecord
		if err := rows.Scan(&r.ClientID, &r.DisplayName, &r.SentAt); err != nil {
			return nil, err
		}
		requests = append(requests, r)
	}

	return requests, rows.Err()
}
