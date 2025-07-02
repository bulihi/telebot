package main

import (
	"database/sql"
	"fmt"
	"log"
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

type Message struct {
	ID             int64     `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	ChatID         int64     `json:"chat_id"`
	ChatTitle      string    `json:"chat_title"`
	UserName       string    `json:"user_name"`
	FromUserID     int64     `json:"from_user_id"`
	MessageType    string    `json:"message_type"`
	MessageContent string    `json:"message_content"`
	FilePath       string    `json:"file_path,omitempty"`
}

type Chat struct {
	ChatID    int64     `json:"chat_id"`
	Title     string    `json:"title"`
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
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

	// 创建群组表
	chatSchema := `
	CREATE TABLE IF NOT EXISTS chats (
		chat_id INTEGER PRIMARY KEY,
		title TEXT NOT NULL,
		type TEXT NOT NULL,
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

	// 创建消息表
	messageSchema := `
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		chat_id INTEGER NOT NULL,
		chat_title TEXT NOT NULL,
		user_name TEXT NOT NULL,
		from_user_id INTEGER NOT NULL,
		message_type TEXT NOT NULL,
		message_content TEXT,
		file_path TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON messages(chat_id);
	CREATE INDEX IF NOT EXISTS idx_messages_timestamp ON messages(timestamp);`

	_, err := d.db.Exec(keywordSchema)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(violationSchema)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(chatSchema)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(groupSettingsSchema)
	if err != nil {
		return err
	}

	_, err = d.db.Exec(messageSchema)
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

// 添加消息记录
func (d *Database) LogMessage(msg *Message) error {
	query := `INSERT INTO messages (
		timestamp, chat_id, chat_title, user_name, from_user_id,
		message_type, message_content, file_path
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := d.db.Exec(query,
		msg.Timestamp,
		msg.ChatID,
		msg.ChatTitle,
		msg.UserName,
		msg.FromUserID,
		msg.MessageType,
		msg.MessageContent,
		msg.FilePath,
	)
	return err
}

// 获取消息列表
func (d *Database) GetMessages(chatID int64, page, perPage int, messageType string) ([]Message, int, error) {
	// 构建基础查询
	baseQuery := `SELECT id, timestamp, chat_id, chat_title, user_name, 
		from_user_id, message_type, message_content, file_path 
		FROM messages WHERE 1=1`
	countQuery := `SELECT COUNT(*) FROM messages WHERE 1=1`
	var params []interface{}

	// 添加过滤条件
	if chatID != 0 {
		baseQuery += ` AND chat_id = ?`
		countQuery += ` AND chat_id = ?`
		params = append(params, chatID)
	}
	if messageType != "all" {
		baseQuery += ` AND message_type = ?`
		countQuery += ` AND message_type = ?`
		params = append(params, messageType)
	}

	// 获取总数
	var total int
	err := d.db.QueryRow(countQuery, params...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	// 添加排序和分页
	baseQuery += ` ORDER BY timestamp DESC LIMIT ? OFFSET ?`
	offset := (page - 1) * perPage
	params = append(params, perPage, offset)

	// 执行主查询
	rows, err := d.db.Query(baseQuery, params...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		err := rows.Scan(
			&msg.ID,
			&msg.Timestamp,
			&msg.ChatID,
			&msg.ChatTitle,
			&msg.UserName,
			&msg.FromUserID,
			&msg.MessageType,
			&msg.MessageContent,
			&msg.FilePath,
		)
		if err != nil {
			return nil, 0, err
		}
		messages = append(messages, msg)
	}

	return messages, total, nil
}

// 获取所有群组列表
func (d *Database) GetAllChats() ([]struct {
	ChatID int64  `json:"chat_id"`
	Title  string `json:"title"`
}, error) {
	query := `SELECT DISTINCT chat_id, chat_title FROM messages ORDER BY chat_title`
	rows, err := d.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []struct {
		ChatID int64  `json:"chat_id"`
		Title  string `json:"title"`
	}

	for rows.Next() {
		var chat struct {
			ChatID int64  `json:"chat_id"`
			Title  string `json:"title"`
		}
		if err := rows.Scan(&chat.ChatID, &chat.Title); err != nil {
			return nil, err
		}
		chats = append(chats, chat)
	}

	return chats, nil
}

// 迁移数据库结构
func (d *Database) Migrate() error {
	// 1. 检查是否需要添加新的列
	// keywords 表
	columns, err := d.getTableColumns("keywords")
	if err != nil {
		return err
	}

	if !containsColumn(columns, "is_active") {
		_, err = d.db.Exec(`ALTER TABLE keywords ADD COLUMN is_active BOOLEAN DEFAULT 1;`)
		if err != nil {
			return err
		}
		log.Printf("✅ 已添加 keywords.is_active 列")
	}

	// 2. 创建新表（如果不存在）
	// chats 表
	if !d.tableExists("chats") {
		chatSchema := `
		CREATE TABLE chats (
			chat_id INTEGER PRIMARY KEY,
			title TEXT NOT NULL,
			type TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`
		_, err = d.db.Exec(chatSchema)
		if err != nil {
			return err
		}
		log.Printf("✅ 已创建 chats 表")
	}

	// group_settings 表
	if !d.tableExists("group_settings") {
		groupSettingsSchema := `
		CREATE TABLE group_settings (
			chat_id INTEGER PRIMARY KEY,
			welcome_message TEXT,
			verification_enabled BOOLEAN DEFAULT 1,
			question TEXT,
			answer TEXT,
			timeout INTEGER DEFAULT 300,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`
		_, err = d.db.Exec(groupSettingsSchema)
		if err != nil {
			return err
		}
		log.Printf("✅ 已创建 group_settings 表")
	}

	// violations 表
	if !d.tableExists("violations") {
		violationSchema := `
		CREATE TABLE violations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			username TEXT,
			chat_id INTEGER NOT NULL,
			message_text TEXT,
			keyword TEXT NOT NULL,
			action TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);`
		_, err = d.db.Exec(violationSchema)
		if err != nil {
			return err
		}
		log.Printf("✅ 已创建 violations 表")
	}

	log.Printf("✅ 数据库迁移完成")
	return nil
}

// 检查表是否存在
func (d *Database) tableExists(tableName string) bool {
	var name string
	err := d.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", tableName).Scan(&name)
	return err == nil
}

// 获取表的列信息
func (d *Database) getTableColumns(tableName string) ([]string, error) {
	rows, err := d.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid int
		var name string
		var type_ string
		var notnull int
		var dflt_value sql.NullString
		var pk int
		err = rows.Scan(&cid, &name, &type_, &notnull, &dflt_value, &pk)
		if err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, nil
}

// 检查列是否存在
func containsColumn(columns []string, column string) bool {
	for _, c := range columns {
		if c == column {
			return true
		}
	}
	return false
}

func (d *Database) Close() error {
	return d.db.Close()
}

// UpsertChat 更新或插入群组信息
func (d *Database) UpsertChat(chat *Chat) error {
	query := `INSERT INTO chats (chat_id, title, type) 
			  VALUES (?, ?, ?) 
			  ON CONFLICT(chat_id) DO UPDATE SET 
			  title = excluded.title,
			  type = excluded.type`

	_, err := d.db.Exec(query, chat.ChatID, chat.Title, chat.Type)
	return err
}

// GetChat 获取群组信息
func (d *Database) GetChat(chatID int64) (*Chat, error) {
	query := `SELECT chat_id, title, type, created_at FROM chats WHERE chat_id = ?`

	var chat Chat
	err := d.db.QueryRow(query, chatID).Scan(
		&chat.ChatID,
		&chat.Title,
		&chat.Type,
		&chat.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &chat, nil
}
