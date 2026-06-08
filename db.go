package main

import (
	"database/sql"
	"fmt"

	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var db *sql.DB

type TokenEntry struct {
	ID           int
	Organization string
	Token        string
	CreatedAt    time.Time
}

func initDB() error {
	dbPath := "/etc/onet/onet.db"

	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create db directory: %v", err)
	}

	var err error
	db, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}

	createTableSQL := `CREATE TABLE IF NOT EXISTS tokens (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		organization TEXT NOT NULL,
		token TEXT NOT NULL UNIQUE,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = db.Exec(createTableSQL)
	if err != nil {
		return fmt.Errorf("failed to create tokens table: %v", err)
	}

	return nil
}

func AddToken(organization, token string) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	insertSQL := `INSERT INTO tokens(organization, token) VALUES (?, ?)`
	_, err := db.Exec(insertSQL, organization, token)
	return err
}

func RemoveToken(token string) error {
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	deleteSQL := `DELETE FROM tokens WHERE token = ?`
	res, err := db.Exec(deleteSQL, token)
	if err != nil {
		return err
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

func ListTokens() ([]TokenEntry, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	rows, err := db.Query("SELECT id, organization, token, created_at FROM tokens ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []TokenEntry
	for rows.Next() {
		var t TokenEntry
		if err := rows.Scan(&t.ID, &t.Organization, &t.Token, &t.CreatedAt); err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, nil
}

func VerifyToken(token string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM tokens WHERE token = ?)`
	err := db.QueryRow(query, token).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return exists, nil
}
