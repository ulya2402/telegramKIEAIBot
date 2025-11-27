package database

import (
	"database/sql"
	"encoding/json"
	"sync"

	_ "modernc.org/sqlite"
)

type SQLiteDB struct {
	DB *sql.DB
	mu sync.Mutex
}

type UserState struct {
	State        string
	SelectedModel string
	DraftOptions  map[string]interface{}
}

func NewSQLiteDB(dbPath string) (*SQLiteDB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	instance := &SQLiteDB{DB: db}
	if err := instance.initTables(); err != nil {
		return nil, err
	}

	return instance, nil
}

func (s *SQLiteDB) initTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			user_id INTEGER PRIMARY KEY,
			language_code TEXT NOT NULL DEFAULT 'en',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS user_states (
			user_id INTEGER PRIMARY KEY,
			state TEXT DEFAULT 'IDLE',
			selected_model TEXT DEFAULT '',
			draft_options TEXT DEFAULT '{}'
		);`,
	}

	for _, q := range queries {
		if _, err := s.DB.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteDB) SetUserLanguage(userID int64, langCode string) error {
	query := `INSERT INTO users (user_id, language_code) VALUES (?, ?)
			  ON CONFLICT(user_id) DO UPDATE SET language_code = excluded.language_code;`
	_, err := s.DB.Exec(query, userID, langCode)
	return err
}

func (s *SQLiteDB) GetUserLanguage(userID int64) string {
	query := `SELECT language_code FROM users WHERE user_id = ?`
	var langCode string
	err := s.DB.QueryRow(query, userID).Scan(&langCode)
	if err != nil {
		return "en" 
	}
	return langCode
}

func (s *SQLiteDB) SetUserState(userID int64, state string, modelID string) error {
	query := `INSERT INTO user_states (user_id, state, selected_model) VALUES (?, ?, ?)
			  ON CONFLICT(user_id) DO UPDATE SET state = excluded.state, selected_model = excluded.selected_model;`
	_, err := s.DB.Exec(query, userID, state, modelID)
	return err
}

func (s *SQLiteDB) UpdateDraftOption(userID int64, key string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	currentState := s.GetUserState(userID)
	if currentState.DraftOptions == nil {
		currentState.DraftOptions = make(map[string]interface{})
	}
	currentState.DraftOptions[key] = value

	jsonBytes, _ := json.Marshal(currentState.DraftOptions)
	query := `UPDATE user_states SET draft_options = ? WHERE user_id = ?`
	_, err := s.DB.Exec(query, string(jsonBytes), userID)
	return err
}

func (s *SQLiteDB) GetUserState(userID int64) UserState {
	query := `SELECT state, selected_model, draft_options FROM user_states WHERE user_id = ?`
	var state, model, optionsRaw string
	err := s.DB.QueryRow(query, userID).Scan(&state, &model, &optionsRaw)
	if err != nil {
		return UserState{State: "IDLE", DraftOptions: make(map[string]interface{})}
	}

	var options map[string]interface{}
	json.Unmarshal([]byte(optionsRaw), &options)
	if options == nil {
		options = make(map[string]interface{})
	}

	return UserState{
		State:        state,
		SelectedModel: model,
		DraftOptions:  options,
	}
}

func (s *SQLiteDB) Close() {
	if s.DB != nil {
		s.DB.Close()
	}
}