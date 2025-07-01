package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
)

type WebServer struct {
	config     *Config
	db         *Database
	store      *sessions.CookieStore
	reloadChan chan struct{} // 添加重载通道
}

func NewWebServer(config *Config, db *Database, reloadChan chan struct{}) *WebServer {
	// 使用配置的密码作为session密钥
	store := sessions.NewCookieStore([]byte(config.Server.AdminPassword))

	// 配置 cookie
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7, // 7天
		HttpOnly: true,
		Secure:   false, // 如果使用HTTPS，设置为true
		SameSite: http.SameSiteLaxMode,
	}

	return &WebServer{
		config:     config,
		db:         db,
		store:      store,
		reloadChan: reloadChan,
	}
}

func (ws *WebServer) Start() error {
	r := mux.NewRouter()

	// 静态文件
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// 登录相关
	r.HandleFunc("/login", ws.handleLogin).Methods("GET", "POST")
	r.HandleFunc("/logout", ws.handleLogout)

	// API路由
	api := r.PathPrefix("/api").Subrouter()
	api.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ws.authMiddleware(next.ServeHTTP).ServeHTTP(w, r)
		})
	})
	api.HandleFunc("/keywords", ws.handleAPIKeywords).Methods("GET", "POST")
	api.HandleFunc("/keywords/{id:[0-9]+}", ws.handleAPIDeleteKeyword).Methods("DELETE")
	api.HandleFunc("/reload", ws.handleAPIReload).Methods("POST")
	api.HandleFunc("/group-settings/{chatID}", ws.handleAPIGroupSettings).Methods("GET", "POST")

	// 页面路由
	r.HandleFunc("/", ws.authMiddleware(ws.handleDashboard))
	r.HandleFunc("/keywords", ws.authMiddleware(ws.handleKeywords))
	r.HandleFunc("/violations", ws.authMiddleware(ws.handleViolations))
	r.HandleFunc("/group-settings", ws.authMiddleware(ws.handleGroupSettingsPage))

	return http.ListenAndServe(ws.config.Server.Port, r)
}

func (ws *WebServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := ws.store.Get(r, "admin-session")
		if err != nil {
			http.Error(w, "Session错误", http.StatusInternalServerError)
			return
		}

		if auth, ok := session.Values["authenticated"].(bool); !ok || !auth {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (ws *WebServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	session, err := ws.store.Get(r, "admin-session")
	if err != nil {
		http.Error(w, "Session错误", http.StatusInternalServerError)
		return
	}

	if r.Method == "POST" {
		password := r.FormValue("password")
		if password == ws.config.Server.AdminPassword {
			session.Values["authenticated"] = true
			err = session.Save(r, w)
			if err != nil {
				http.Error(w, "保存Session失败", http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	// 如果已经登录，直接跳转到首页
	if auth, ok := session.Values["authenticated"].(bool); ok && auth {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>登录 - Telegram Bot 管理面板</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 0; padding: 0; background: #f5f5f5; }
        .login-container { 
            max-width: 400px; 
            margin: 100px auto; 
            padding: 20px;
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
        }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; margin-bottom: 5px; }
        .form-group input { 
            width: 100%; 
            padding: 8px; 
            border: 1px solid #ddd; 
            border-radius: 4px;
            box-sizing: border-box;
        }
        .btn {
            background: #007bff;
            color: white;
            padding: 10px 20px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            width: 100%;
        }
        .btn:hover { background: #0056b3; }
        h1 { text-align: center; color: #333; }
        .error-message {
            color: #dc3545;
            margin-bottom: 15px;
            text-align: center;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h1>管理员登录</h1>
        <form method="POST" action="/login">
            <div class="form-group">
                <label>密码:</label>
                <input type="password" name="password" required>
            </div>
            <button type="submit" class="btn">登录</button>
        </form>
    </div>
</body>
</html>`

	t := template.Must(template.New("login").Parse(tmpl))
	t.Execute(w, nil)
}

func (ws *WebServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	session, err := ws.store.Get(r, "admin-session")
	if err != nil {
		http.Error(w, "Session错误", http.StatusInternalServerError)
		return
	}

	session.Values["authenticated"] = false
	session.Options.MaxAge = -1 // 使cookie立即过期
	err = session.Save(r, w)
	if err != nil {
		http.Error(w, "保存Session失败", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (ws *WebServer) handleAPIReload(w http.ResponseWriter, r *http.Request) {
	// 重新加载关键词
	keywords, err := ws.db.GetKeywords()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 通知bot重新加载关键词
	select {
	case ws.reloadChan <- struct{}{}:
		log.Printf("已发送重载信号")
	default:
		log.Printf("重载通道已满，跳过发送信号")
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "关键词已重新加载",
		"count":   len(keywords),
	})
}

func (ws *WebServer) handleAPIGroupSettings(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	chatID, err := strconv.ParseInt(vars["chatID"], 10, 64)
	if err != nil {
		http.Error(w, "无效的群组ID", http.StatusBadRequest)
		return
	}

	if r.Method == "GET" {
		settings, err := ws.db.GetGroupSettings(chatID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(settings)
		return
	}

	if r.Method == "POST" {
		var settings GroupSettings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		settings.ChatID = chatID
		if err := ws.db.UpdateGroupSettings(&settings); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}

// 仪表板页面
func (ws *WebServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	keywords, _ := ws.db.GetKeywords()
	violations, _ := ws.db.GetViolations(10)

	data := struct {
		KeywordCount     int
		ViolationCount   int
		RecentViolations []Violation
	}{
		KeywordCount:     len(keywords),
		ViolationCount:   len(violations),
		RecentViolations: violations,
	}

	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Telegram Bot 管理面板</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .header { text-align: center; margin-bottom: 30px; }
        .stats { display: flex; justify-content: space-around; margin-bottom: 30px; }
        .stat-card { background: #007bff; color: white; padding: 20px; border-radius: 8px; text-align: center; }
        .nav { margin-bottom: 20px; }
        .nav a { margin-right: 20px; text-decoration: none; color: #007bff; }
        .nav a:hover { text-decoration: underline; }
        table { width: 100%; border-collapse: collapse; margin-top: 20px; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f8f9fa; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🤖 Telegram Bot 管理面板</h1>
        </div>
        
        <div class="nav">
            <a href="/">仪表板</a>
            <a href="/keywords">关键词管理</a>
            <a href="/violations">违规记录</a>
        </div>
        
        <div class="stats">
            <div class="stat-card">
                <h3>{{.KeywordCount}}</h3>
                <p>关键词总数</p>
            </div>
            <div class="stat-card">
                <h3>{{.ViolationCount}}</h3>
                <p>最近违规</p>
            </div>
        </div>
        
        <h3>最近违规记录</h3>
        <table>
            <thead>
                <tr>
                    <th>用户</th>
                    <th>消息</th>
                    <th>关键词</th>
                    <th>动作</th>
                    <th>时间</th>
                </tr>
            </thead>
            <tbody>
                {{range .RecentViolations}}
                <tr>
                    <td>{{.Username}} ({{.UserID}})</td>
                    <td>{{.MessageText}}</td>
                    <td>{{.Keyword}}</td>
                    <td>{{.Action}}</td>
                    <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>`

	t := template.Must(template.New("dashboard").Parse(tmpl))
	t.Execute(w, data)
}

// 关键词管理页面
func (ws *WebServer) handleKeywords(w http.ResponseWriter, r *http.Request) {
	keywords, _ := ws.db.GetKeywords()

	data := struct {
		Keywords []Keyword
	}{
		Keywords: keywords,
	}

	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>关键词管理 - Telegram Bot</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .nav { margin-bottom: 20px; }
        .nav a { margin-right: 20px; text-decoration: none; color: #007bff; }
        .add-form { background: #f8f9fa; padding: 20px; border-radius: 8px; margin-bottom: 20px; }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; margin-bottom: 5px; font-weight: bold; }
        .form-group input, .form-group select { width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; }
        .btn { background: #007bff; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
        .btn:hover { background: #0056b3; }
        .btn-danger { background: #dc3545; }
        .btn-danger:hover { background: #c82333; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f8f9fa; }
    </style>
</head>
<body>
    <div class="container">
        <h1>关键词管理</h1>
        
        <div class="nav">
            <a href="/">仪表板</a>
            <a href="/keywords">关键词管理</a>
            <a href="/violations">违规记录</a>
        </div>
        
        <div class="add-form">
            <h3>添加新关键词</h3>
            <form id="addKeywordForm">
                <div class="form-group">
                    <label>关键词:</label>
                    <input type="text" id="keyword" required>
                </div>
                <div class="form-group">
                    <label>匹配类型:</label>
                    <select id="matchType">
                        <option value="exact">精确匹配</option>
                        <option value="fuzzy">模糊匹配</option>
                        <option value="regex">正则表达式</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>动作:</label>
                    <select id="action">
                        <option value="mute">禁言</option>
                        <option value="kick">踢出</option>
                    </select>
                </div>
                <button type="submit" class="btn">添加关键词</button>
            </form>
        </div>
        
        <table>
            <thead>
                <tr>
                    <th>ID</th>
                    <th>关键词</th>
                    <th>匹配类型</th>
                    <th>动作</th>
                    <th>创建时间</th>
                    <th>操作</th>
                </tr>
            </thead>
            <tbody>
                {{range .Keywords}}
                <tr>
                    <td>{{.ID}}</td>
                    <td>{{.Keyword}}</td>
                    <td>{{.MatchType}}</td>
                    <td>{{.Action}}</td>
                    <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
                    <td>
                        <button class="btn btn-danger" onclick="deleteKeyword({{.ID}})">删除</button>
                    </td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
    
    <script>
        document.getElementById('addKeywordForm').addEventListener('submit', function(e) {
            e.preventDefault();
            
            const keyword = document.getElementById('keyword').value;
            const matchType = document.getElementById('matchType').value;
            const action = document.getElementById('action').value;
            
            fetch('/api/keywords', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    keyword: keyword,
                    match_type: matchType,
                    action: action
                })
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    location.reload();
                } else {
                    alert('添加失败: ' + data.error);
                }
            });
        });
        
        function deleteKeyword(id) {
            if (confirm('确定要删除这个关键词吗？')) {
                fetch('/api/keywords/' + id, {
                    method: 'DELETE'
                })
                .then(response => response.json())
                .then(data => {
                    if (data.success) {
                        location.reload();
                    } else {
                        alert('删除失败: ' + data.error);
                    }
                });
            }
        }
    </script>
</body>
</html>`

	t := template.Must(template.New("keywords").Parse(tmpl))
	t.Execute(w, data)
}

// 违规记录页面
func (ws *WebServer) handleViolations(w http.ResponseWriter, r *http.Request) {
	violations, _ := ws.db.GetViolations(50)

	data := struct {
		Violations []Violation
	}{
		Violations: violations,
	}

	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>违规记录 - Telegram Bot</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .nav { margin-bottom: 20px; }
        .nav a { margin-right: 20px; text-decoration: none; color: #007bff; }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 12px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background-color: #f8f9fa; }
        .message-text { max-width: 200px; word-break: break-word; }
    </style>
</head>
<body>
    <div class="container">
        <h1>违规记录</h1>
        
        <div class="nav">
            <a href="/">仪表板</a>
            <a href="/keywords">关键词管理</a>
            <a href="/violations">违规记录</a>
        </div>
        
        <table>
            <thead>
                <tr>
                    <th>ID</th>
                    <th>用户ID</th>
                    <th>用户名</th>
                    <th>群组ID</th>
                    <th>消息内容</th>
                    <th>触发关键词</th>
                    <th>执行动作</th>
                    <th>时间</th>
                </tr>
            </thead>
            <tbody>
                {{range .Violations}}
                <tr>
                    <td>{{.ID}}</td>
                    <td>{{.UserID}}</td>
                    <td>{{.Username}}</td>
                    <td>{{.ChatID}}</td>
                    <td class="message-text">{{.MessageText}}</td>
                    <td>{{.Keyword}}</td>
                    <td>{{.Action}}</td>
                    <td>{{.CreatedAt.Format "2006-01-02 15:04:05"}}</td>
                </tr>
                {{end}}
            </tbody>
        </table>
    </div>
</body>
</html>`

	t := template.Must(template.New("violations").Parse(tmpl))
	t.Execute(w, data)
}

func (ws *WebServer) handleGroupSettingsPage(w http.ResponseWriter, r *http.Request) {
	tmpl := `<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>群组设置 - Telegram Bot</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background-color: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        .nav { margin-bottom: 20px; }
        .nav a { margin-right: 20px; text-decoration: none; color: #007bff; }
        .form-group { margin-bottom: 15px; }
        .form-group label { display: block; margin-bottom: 5px; }
        .form-group input, .form-group textarea { width: 100%; padding: 8px; border: 1px solid #ddd; border-radius: 4px; }
        .form-group textarea { height: 150px; }
        .btn { background: #007bff; color: white; padding: 10px 20px; border: none; border-radius: 4px; cursor: pointer; }
        .btn:hover { background: #0056b3; }
    </style>
</head>
<body>
    <div class="container">
        <h1>群组设置</h1>
        
        <div class="nav">
            <a href="/">仪表板</a>
            <a href="/keywords">关键词管理</a>
            <a href="/violations">违规记录</a>
            <a href="/group-settings">群组设置</a>
        </div>

        <div id="settings-form">
            <div class="form-group">
                <label>群组ID:</label>
                <input type="number" id="chatID" required>
            </div>
            <div class="form-group">
                <label>欢迎消息:</label>
                <textarea id="welcomeMessage"></textarea>
            </div>
            <div class="form-group">
                <label>
                    <input type="checkbox" id="verificationEnabled">
                    启用验证
                </label>
            </div>
            <div class="form-group">
                <label>验证问题:</label>
                <input type="text" id="question">
            </div>
            <div class="form-group">
                <label>正确答案:</label>
                <input type="text" id="answer">
            </div>
            <div class="form-group">
                <label>超时时间(秒):</label>
                <input type="number" id="timeout" value="300">
            </div>
            <button class="btn" onclick="saveSettings()">保存设置</button>
        </div>
    </div>

    <script>
        function loadSettings(chatID) {
            fetch('/api/group-settings/' + chatID)
                .then(response => response.json())
                .then(data => {
                    document.getElementById('welcomeMessage').value = data.welcome_message || '';
                    document.getElementById('verificationEnabled').checked = data.verification_enabled;
                    document.getElementById('question').value = data.question || '';
                    document.getElementById('answer').value = data.answer || '';
                    document.getElementById('timeout').value = data.timeout || 300;
                });
        }

        function saveSettings() {
            const chatID = document.getElementById('chatID').value;
            const data = {
                welcome_message: document.getElementById('welcomeMessage').value,
                verification_enabled: document.getElementById('verificationEnabled').checked,
                question: document.getElementById('question').value,
                answer: document.getElementById('answer').value,
                timeout: parseInt(document.getElementById('timeout').value)
            };

            fetch('/api/group-settings/' + chatID, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(data)
            })
            .then(response => response.json())
            .then(data => {
                if (data.success) {
                    alert('设置已保存');
                } else {
                    alert('保存失败: ' + data.error);
                }
            });
        }

        // 当输入群组ID时加载设置
        document.getElementById('chatID').addEventListener('change', function() {
            loadSettings(this.value);
        });
    </script>
</body>
</html>`

	t := template.Must(template.New("group-settings").Parse(tmpl))
	t.Execute(w, nil)
}

func (ws *WebServer) handleAPIDeleteKeyword(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, err := strconv.Atoi(vars["id"])
	if err != nil {
		http.Error(w, "无效的ID", http.StatusBadRequest)
		return
	}

	err = ws.db.DeleteKeyword(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (ws *WebServer) handleAPIKeywords(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		keywords, err := ws.db.GetKeywords()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(keywords)
		return
	}

	if r.Method == "POST" {
		var keyword struct {
			Keyword   string `json:"keyword"`
			MatchType string `json:"match_type"`
			Action    string `json:"action"`
		}

		if err := json.NewDecoder(r.Body).Decode(&keyword); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err := ws.db.AddKeyword(keyword.Keyword, keyword.MatchType, keyword.Action)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]bool{"success": true})
	}
}
