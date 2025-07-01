package main

import (
	"database/sql"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Database struct {
	db *sql.DB
}

type Keyword struct {
	ID        int       `json:"id"`
	Keyword   string    `json:"keyword"`
	MatchType string    `json:"match_type"` // exact, fuzzy, regex
	Action    string    `json:"action"`     // mute, kick
	CreatedAt time.Time `json:"created_at"`
	IsActive  bool      `json:"is_active"`
}

type Violation struct {
	ID          int       `json:"id"`
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	ChatID      int64     `json:"chat_id"`
	MessageText string    `json:"message_text"`
	Keyword     string    `json:"keyword"`
	Action      string    `json:"action"`
	CreatedAt   time.Time `json:"created_at"`
}

type GroupSettings struct {
	ChatID              int64     `json:"chat_id"`
	WelcomeMessage      string    `json:"welcome_message"`
	VerificationEnabled bool      `json:"verification_enabled"`
	Question            string    `json:"question"`
	Answer              string    `json:"answer"`
	Timeout             int       `json:"timeout"`
	UpdatedAt           time.Time `json:"updated_at"`
}

func NewDatabase(dbPath string) (*Database, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	d := &Database{db: db}
	err = d.createTables()
	if err != nil {
		return nil, err
	}

	return d, nil
}

func (d *Database) createTables() error {
	// 创建关键词表
	keywordSchema := `
	CREATE TABLE IF NOT EXISTS keywords (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		keyword TEXT NOT NULL,
		match_type TEXT NOT NULL DEFAULT 'exact',
		action TEXT NOT NULL DEFAULT 'mute',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		is_active BOOLEAN DEFAULT 1
	);`

	// 创建违规记录表
	violationSchema := `
	CREATE TABLE IF NOT EXISTS violations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_id INTEGER NOT NULL,
		username TEXT,
		chat_id INTEGER NOT NULL,
		message_text TEXT,
		keyword TEXT NOT NULL,
		action TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	// 创建群组设置表
	groupSettingsSchema := `
	CREATE TABLE IF NOT EXISTS group_settings (
		chat_id INTEGER PRIMARY KEY,
		welcome_message TEXT,
		verification_enabled BOOLEAN DEFAULT 1,
		question TEXT,
		answer TEXT,
		timeout INTEGER DEFAULT 300,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err := d.db.Exec(keywordSchema)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(violationSchema)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(groupSettingsSchema)
	if err != nil {
		return err
	}

	return nil
}

// 关键词管理
func (d *Database) AddKeyword(keyword, matchType, action string) error {
	query := `INSERT INTO keywords (keyword, match_type, action) VALUES (?, ?, ?)`
	_, err := d.db.Exec(query, keyword, matchType, action)
	return err
}

func (d *Database) GetKeywords() ([]Keyword, error) {
	query := `SELECT id, keyword, match_type, action, created_at, is_active FROM keywords WHERE is_active = 1`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keywords []Keyword
	for rows.Next() {
		var k Keyword
		err := rows.Scan(&k.ID, &k.Keyword, &k.MatchType, &k.Action, &k.CreatedAt, &k.IsActive)
		if err != nil {
			return nil, err
		}
		keywords = append(keywords, k)
	}

	return keywords, nil
}

func (d *Database) DeleteKeyword(id int) error {
	query := `UPDATE keywords SET is_active = 0 WHERE id = ?`
	_, err := d.db.Exec(query, id)
	return err
}

// 违规记录
func (d *Database) LogViolation(userID int64, username string, chatID int64, messageText, keyword, action string) error {
	query := `INSERT INTO violations (user_id, username, chat_id, message_text, keyword, action) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := d.db.Exec(query, userID, username, chatID, messageText, keyword, action)
	return err
}

func (d *Database) GetViolations(limit int) ([]Violation, error) {
	query := `SELECT id, user_id, username, chat_id, message_text, keyword, action, created_at 
			  FROM violations ORDER BY created_at DESC LIMIT ?`
	rows, err := d.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var violations []Violation
	for rows.Next() {
		var v Violation
		var username sql.NullString
		err := rows.Scan(&v.ID, &v.UserID, &username, &v.ChatID, &v.MessageText, &v.Keyword, &v.Action, &v.CreatedAt)
		if err != nil {
			return nil, err
		}
		if username.Valid {
			v.Username = username.String
		}
		violations = append(violations, v)
	}

	return violations, nil
}

// 群组设置相关函数
func (d *Database) GetGroupSettings(chatID int64) (*GroupSettings, error) {
	query := `SELECT chat_id, welcome_message, verification_enabled, question, answer, timeout, updated_at 
			  FROM group_settings WHERE chat_id = ?`

	var settings GroupSettings
	err := d.db.QueryRow(query, chatID).Scan(
		&settings.ChatID,
		&settings.WelcomeMessage,
		&settings.VerificationEnabled,
		&settings.Question,
		&settings.Answer,
		&settings.Timeout,
		&settings.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &settings, nil
}

func (d *Database) UpdateGroupSettings(settings *GroupSettings) error {
	query := `INSERT OR REPLACE INTO group_settings 
			  (chat_id, welcome_message, verification_enabled, question, answer, timeout, updated_at)
			  VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`

	_, err := d.db.Exec(query,
		settings.ChatID,
		settings.WelcomeMessage,
		settings.VerificationEnabled,
		settings.Question,
		settings.Answer,
		settings.Timeout,
	)

	return err
}

func (d *Database) Close() error {
	return d.db.Close()
}
