# Telegram 群组消息管理机器人

这是一个用Golang开发的Telegram群组消息管理机器人，可以自动检测和处理违规消息。

## 功能特性

- 🤖 **自动监控群组消息**
  - 支持文字消息检测
  - 支持链接内容检测
  - 支持图片/文件名检测
  - 支持图片描述检测

- 🔍 **多种匹配模式**
  - 精确匹配 (exact)
  - 模糊匹配 (fuzzy)
  - 正则表达式匹配 (regex)

- ⚡ **自动处理违规用户**
  - 禁言用户 (可设置时长)
  - 踢出用户
  - 自动删除违规消息

- 📊 **Web管理界面**
  - 关键词管理
  - 违规记录查看
  - 实时统计面板

- 💾 **数据存储**
  - SQLite数据库存储
  - 违规记录日志
  - 关键词管理

## 安装使用

### 1. 准备工作

1. 创建Telegram Bot并获取Token：
   - 联系 [@BotFather](https://t.me/botfather)
   - 使用 `/newbot` 命令创建新机器人
   - 获取Bot Token

2. 获取管理员用户ID：
   - 联系 [@userinfobot](https://t.me/userinfobot)
   - 发送任意消息获取你的用户ID

3. 将机器人添加到群组并设置为管理员

### 2. 配置文件

编辑 `config.yaml` 文件：

```yaml
telegram:
  bot_token: "YOUR_BOT_TOKEN_HERE"  # 替换为你的Bot Token
  admin_user_id: 0                  # 替换为你的用户ID

database:
  path: "bot.db"

server:
  port: ":8080"
  
settings:
  default_action: "mute"  # mute 或 kick
  mute_duration: 3600     # 禁言时长（秒）
  log_violations: true    # 是否记录违规日志
```

### 3. 运行程序

```bash
# 下载依赖
go mod tidy

# 编译运行
go run .

# 或者编译后运行
go build -o telegramBot.exe
./telegramBot.exe
```

### 4. 访问Web管理界面

在浏览器中访问：`http://localhost:8080`

## Bot命令

### 管理员命令（私聊或群组中使用）

- `/start` 或 `/help` - 显示帮助信息
- `/add_keyword <关键词> <匹配类型> <动作>` - 添加关键词
- `/list_keywords` - 查看所有关键词
- `/delete_keyword <ID>` - 删除关键词
- `/violations [数量]` - 查看违规记录
- `/reload` - 重新加载关键词
- `/status` - 查看机器人状态

### 命令示例

```bash
# 添加精确匹配关键词，触发时禁言
/add_keyword 违规词 exact mute

# 添加模糊匹配关键词，触发时踢出
/add_keyword 广告 fuzzy kick

# 添加正则表达式匹配，检测链接
/add_keyword "https?://.*\\.com" regex mute

# 查看最近20条违规记录
/violations 20

# 删除ID为1的关键词
/delete_keyword 1
```

## 匹配类型说明

1. **精确匹配 (exact)**
   - 完全匹配关键词
   - 支持作为独立单词匹配
   - 适用于特定词汇封禁

2. **模糊匹配 (fuzzy)**
   - 包含关键词即匹配
   - 适用于广泛内容过滤

3. **正则表达式 (regex)**
   - 使用正则表达式模式
   - 支持复杂匹配规则
   - 适用于链接、邮箱等格式检测

## 处理动作

- **mute** - 禁言用户，时长在配置文件中设置
- **kick** - 踢出用户

## Web界面功能

- **仪表板** - 查看总体统计和最近违规
- **关键词管理** - 添加、删除关键词
- **违规记录** - 查看详细违规历史

## 目录结构

```
telegramBot/
├── main.go          # 主程序入口
├── config.go        # 配置管理
├── database.go      # 数据库操作
├── bot.go           # Telegram Bot逻辑
├── filter.go        # 消息过滤器
├── web.go           # Web管理界面
├── config.yaml      # 配置文件
├── go.mod           # Go模块文件
└── README.md        # 说明文档
```

## 注意事项

1. 确保机器人在群组中具有管理员权限
2. 定期备份数据库文件 `bot.db`
3. 机器人只在群组中工作，私聊仅用于管理命令
4. 正则表达式要谨慎使用，避免误伤正常消息

## 技术栈

- Go 1.23.4
- Telegram Bot API
- SQLite 数据库
- Gorilla Mux (Web路由)
- HTML/CSS/JavaScript (前端)

## 问题排查

如果遇到问题：

1. 检查配置文件是否正确设置
2. 确认机器人Token有效
3. 确认机器人有群组管理员权限
4. 查看终端日志输出

## 许可证

MIT License # telebot
