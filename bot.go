package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramBot struct {
	bot    *tgbotapi.BotAPI
	config *Config
	db     *Database
	filter *MessageFilter
	// 添加验证状态管理
	verificationStatus map[int64]map[int64]*VerificationStatus
}

type VerificationStatus struct {
	UserID    int64
	ChatID    int64
	StartTime time.Time
	Attempts  int
}

func NewTelegramBot(config *Config, db *Database) (*TelegramBot, error) {
	var httpClient *http.Client

	// 配置代理（如果启用）
	if config.Proxy.Enabled && config.Proxy.URL != "" {
		log.Printf("🔄 正在配置代理: %s", config.Proxy.URL)
		proxyURL, err := url.Parse(config.Proxy.URL)
		if err != nil {
			log.Printf("⚠️ 代理URL解析失败: %v", err)
		} else {
			// 增加超时时间
			httpClient = &http.Client{
				Transport: &http.Transport{
					Proxy:                 http.ProxyURL(proxyURL),
					ResponseHeaderTimeout: time.Second * 60,
					IdleConnTimeout:       time.Second * 90,
					TLSHandshakeTimeout:   time.Second * 30,
				},
				Timeout: time.Second * 90, // 增加总体超时时间
			}
			log.Printf("✅ 已启用代理: %s", config.Proxy.URL)
		}
	} else {
		log.Printf("⚠️ 未配置代理")
	}

	// 使用配置的代理创建Bot
	var bot *tgbotapi.BotAPI
	var err error

	log.Printf("🔄 正在连接到Telegram API...")
	if httpClient != nil {
		log.Printf("📡 使用代理客户端连接...")
		bot, err = tgbotapi.NewBotAPIWithClient(config.Telegram.BotToken, tgbotapi.APIEndpoint, httpClient)
	} else {
		log.Printf("📡 使用默认客户端连接...")
		bot, err = tgbotapi.NewBotAPI(config.Telegram.BotToken)
	}
	if err != nil {
		log.Printf("❌ 连接失败: %v", err)
		return nil, fmt.Errorf("连接Telegram API失败: %v", err)
	}

	// 删除webhook
	log.Printf("🔄 正在删除webhook...")
	_, err = bot.Request(tgbotapi.DeleteWebhookConfig{
		DropPendingUpdates: true,
	})
	if err != nil {
		log.Printf("❌ 删除webhook失败: %v", err)
		return nil, fmt.Errorf("删除webhook失败: %v", err)
	}
	log.Printf("✅ Webhook已删除")

	bot.Debug = true // 开启调试模式

	// 初始化过滤器
	keywords, err := db.GetKeywords()
	if err != nil {
		return nil, err
	}

	filter := NewMessageFilter(keywords)

	tb := &TelegramBot{
		bot:                bot,
		config:             config,
		db:                 db,
		filter:             filter,
		verificationStatus: make(map[int64]map[int64]*VerificationStatus),
	}

	log.Printf("✅ Bot已连接：%s", bot.Self.UserName)
	return tb, nil
}

func (tb *TelegramBot) Start() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := tb.bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil {
			tb.handleMessage(update.Message)
		} else if update.ChatMember != nil {
			// 处理新成员加入
			tb.handleNewMember(update.ChatMember)
		}
	}
}

func (tb *TelegramBot) handleNewMember(chatMember *tgbotapi.ChatMemberUpdated) {
	if chatMember.NewChatMember.Status != "member" {
		return
	}

	settings, err := tb.db.GetGroupSettings(chatMember.Chat.ID)
	if err != nil {
		log.Printf("获取群组设置失败: %v", err)
		return
	}

	// 如果没有特定设置，使用默认设置
	if settings == nil {
		settings = &GroupSettings{
			ChatID:              chatMember.Chat.ID,
			WelcomeMessage:      tb.config.Groups.DefaultSettings.WelcomeMessage,
			VerificationEnabled: tb.config.Groups.DefaultSettings.Verification.Enabled,
			Question:            tb.config.Groups.DefaultSettings.Verification.Question,
			Answer:              tb.config.Groups.DefaultSettings.Verification.Answer,
			Timeout:             tb.config.Groups.DefaultSettings.Verification.Timeout,
		}
	}

	// 发送欢迎消息
	welcomeMsg := strings.ReplaceAll(settings.WelcomeMessage, "{user}", chatMember.From.FirstName)
	welcomeMsg = strings.ReplaceAll(welcomeMsg, "{group_name}", chatMember.Chat.Title)
	msg := tgbotapi.NewMessage(chatMember.Chat.ID, welcomeMsg)
	tb.bot.Send(msg)

	// 如果启用了验证
	if settings.VerificationEnabled {
		// 限制用户权限
		restrictConfig := tgbotapi.RestrictChatMemberConfig{
			ChatMemberConfig: tgbotapi.ChatMemberConfig{
				ChatID: chatMember.Chat.ID,
				UserID: chatMember.From.ID,
			},
			UntilDate: time.Now().Add(time.Duration(settings.Timeout) * time.Second).Unix(),
			Permissions: &tgbotapi.ChatPermissions{
				CanSendMessages:       false,
				CanSendMediaMessages:  false,
				CanSendPolls:          false,
				CanSendOtherMessages:  false,
				CanAddWebPagePreviews: false,
				CanChangeInfo:         false,
				CanInviteUsers:        false,
				CanPinMessages:        false,
			},
		}
		tb.bot.Request(restrictConfig)

		// 发送验证问题
		verifyMsg := fmt.Sprintf("欢迎 %s！\n为了防止机器人，请回答以下问题：\n\n%s",
			chatMember.From.FirstName, settings.Question)
		msg := tgbotapi.NewMessage(chatMember.Chat.ID, verifyMsg)
		tb.bot.Send(msg)

		// 记录验证状态
		if _, exists := tb.verificationStatus[chatMember.Chat.ID]; !exists {
			tb.verificationStatus[chatMember.Chat.ID] = make(map[int64]*VerificationStatus)
		}
		tb.verificationStatus[chatMember.Chat.ID][chatMember.From.ID] = &VerificationStatus{
			UserID:    chatMember.From.ID,
			ChatID:    chatMember.Chat.ID,
			StartTime: time.Now(),
			Attempts:  0,
		}

		// 设置超时检查
		go func() {
			time.Sleep(time.Duration(settings.Timeout) * time.Second)
			if status, exists := tb.verificationStatus[chatMember.Chat.ID][chatMember.From.ID]; exists {
				if status.Attempts == 0 {
					// 超时未验证，踢出用户
					kickConfig := tgbotapi.KickChatMemberConfig{
						ChatMemberConfig: tgbotapi.ChatMemberConfig{
							ChatID: chatMember.Chat.ID,
							UserID: chatMember.From.ID,
						},
					}
					tb.bot.Request(kickConfig)
					delete(tb.verificationStatus[chatMember.Chat.ID], chatMember.From.ID)
				}
			}
		}()
	}
}

func (tb *TelegramBot) handleMessage(message *tgbotapi.Message) {
	// 检查是否是验证回答
	if status, exists := tb.verificationStatus[message.Chat.ID][message.From.ID]; exists {
		settings, _ := tb.db.GetGroupSettings(message.Chat.ID)
		if settings == nil {
			settings = &GroupSettings{
				Answer: tb.config.Groups.DefaultSettings.Verification.Answer,
			}
		}

		status.Attempts++
		if message.Text == settings.Answer {
			// 验证成功
			delete(tb.verificationStatus[message.Chat.ID], message.From.ID)

			// 解除限制
			unrestrictConfig := tgbotapi.RestrictChatMemberConfig{
				ChatMemberConfig: tgbotapi.ChatMemberConfig{
					ChatID: message.Chat.ID,
					UserID: message.From.ID,
				},
				Permissions: &tgbotapi.ChatPermissions{
					CanSendMessages:       true,
					CanSendMediaMessages:  true,
					CanSendPolls:          true,
					CanSendOtherMessages:  true,
					CanAddWebPagePreviews: true,
					CanChangeInfo:         false,
					CanInviteUsers:        true,
					CanPinMessages:        false,
				},
			}
			tb.bot.Request(unrestrictConfig)

			successMsg := fmt.Sprintf("✅ 验证成功！欢迎 %s 加入群组！", message.From.FirstName)
			msg := tgbotapi.NewMessage(message.Chat.ID, successMsg)
			tb.bot.Send(msg)
		} else if status.Attempts >= 3 {
			// 验证失败次数过多，踢出用户
			kickConfig := tgbotapi.KickChatMemberConfig{
				ChatMemberConfig: tgbotapi.ChatMemberConfig{
					ChatID: message.Chat.ID,
					UserID: message.From.ID,
				},
			}
			tb.bot.Request(kickConfig)
			delete(tb.verificationStatus[message.Chat.ID], message.From.ID)
		} else {
			// 回答错误，提示重试
			retryMsg := fmt.Sprintf("❌ 回答错误，还有 %d 次机会。", 3-status.Attempts)
			msg := tgbotapi.NewMessage(message.Chat.ID, retryMsg)
			tb.bot.Send(msg)
		}
		return
	}

	// 忽略私聊消息，只处理群组消息
	if !message.Chat.IsGroup() && !message.Chat.IsSuperGroup() {
		tb.handlePrivateMessage(message)
		return
	}

	// 检查管理员命令
	if message.From.ID == tb.config.Telegram.AdminUserID {
		if tb.handleAdminCommand(message) {
			return
		}
	}

	// 检查消息内容
	tb.checkMessageContent(message)
}

func (tb *TelegramBot) handlePrivateMessage(message *tgbotapi.Message) {
	if message.From.ID != tb.config.Telegram.AdminUserID {
		return
	}

	// 处理管理员私聊命令
	tb.handleAdminCommand(message)
}

func (tb *TelegramBot) handleAdminCommand(message *tgbotapi.Message) bool {
	if !message.IsCommand() {
		return false
	}

	command := message.Command()
	args := message.CommandArguments()

	switch command {
	case "start", "help":
		tb.sendHelp(message.Chat.ID)
		return true
	case "add_keyword":
		tb.handleAddKeyword(message.Chat.ID, args)
		return true
	case "list_keywords":
		tb.handleListKeywords(message.Chat.ID)
		return true
	case "delete_keyword":
		tb.handleDeleteKeyword(message.Chat.ID, args)
		return true
	case "violations":
		tb.handleShowViolations(message.Chat.ID, args)
		return true
	case "reload":
		tb.handleReload(message.Chat.ID)
		return true
	case "status":
		tb.handleStatus(message.Chat.ID)
		return true
	}

	return false
}

func (tb *TelegramBot) sendHelp(chatID int64) {
	helpText := `🤖 Telegram群组管理机器人

管理员命令：
/add_keyword <关键词> <匹配类型> <动作> - 添加关键词
  匹配类型：exact(精确), fuzzy(模糊), regex(正则)
  动作：mute(禁言), kick(踢出)
  例：/add_keyword 违规词 fuzzy mute

/list_keywords - 查看所有关键词
/delete_keyword <ID> - 删除关键词
/violations [数量] - 查看违规记录 (默认10条)
/reload - 重新加载关键词
/status - 查看机器人状态
/help - 显示此帮助

功能：
✅ 监控群组消息
✅ 精确/模糊/正则匹配
✅ 检测链接内容
✅ 检测图片文件名和描述
✅ 自动禁言或踢出违规用户
✅ 记录违规日志`

	msg := tgbotapi.NewMessage(chatID, helpText)
	tb.bot.Send(msg)
}

func (tb *TelegramBot) handleAddKeyword(chatID int64, args string) {
	parts := strings.Fields(args)
	if len(parts) < 3 {
		msg := tgbotapi.NewMessage(chatID, "❌ 用法：/add_keyword <关键词> <匹配类型> <动作>\n匹配类型：exact, fuzzy, regex\n动作：mute, kick")
		tb.bot.Send(msg)
		return
	}

	keyword := parts[0]
	matchType := parts[1]
	action := parts[2]

	// 验证参数
	if matchType != "exact" && matchType != "fuzzy" && matchType != "regex" {
		msg := tgbotapi.NewMessage(chatID, "❌ 匹配类型必须是：exact, fuzzy, regex")
		tb.bot.Send(msg)
		return
	}

	if action != "mute" && action != "kick" {
		msg := tgbotapi.NewMessage(chatID, "❌ 动作必须是：mute, kick")
		tb.bot.Send(msg)
		return
	}

	// 添加到数据库
	err := tb.db.AddKeyword(keyword, matchType, action)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 添加失败：%v", err))
		tb.bot.Send(msg)
		return
	}

	// 重新加载关键词
	tb.reloadKeywords()

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ 关键词已添加\n关键词：%s\n匹配类型：%s\n动作：%s", keyword, matchType, action))
	tb.bot.Send(msg)
}

func (tb *TelegramBot) handleListKeywords(chatID int64) {
	keywords, err := tb.db.GetKeywords()
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 获取关键词失败：%v", err))
		tb.bot.Send(msg)
		return
	}

	if len(keywords) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📝 暂无关键词")
		tb.bot.Send(msg)
		return
	}

	var text strings.Builder
	text.WriteString("📝 关键词列表：\n\n")

	for _, k := range keywords {
		text.WriteString(fmt.Sprintf("ID: %d\n", k.ID))
		text.WriteString(fmt.Sprintf("关键词: %s\n", k.Keyword))
		text.WriteString(fmt.Sprintf("匹配类型: %s\n", k.MatchType))
		text.WriteString(fmt.Sprintf("动作: %s\n", k.Action))
		text.WriteString(fmt.Sprintf("创建时间: %s\n", k.CreatedAt.Format("2006-01-02 15:04:05")))
		text.WriteString("─────────────\n")
	}

	msg := tgbotapi.NewMessage(chatID, text.String())
	tb.bot.Send(msg)
}

func (tb *TelegramBot) handleDeleteKeyword(chatID int64, args string) {
	if args == "" {
		msg := tgbotapi.NewMessage(chatID, "❌ 用法：/delete_keyword <ID>")
		tb.bot.Send(msg)
		return
	}

	id, err := strconv.Atoi(args)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, "❌ ID必须是数字")
		tb.bot.Send(msg)
		return
	}

	err = tb.db.DeleteKeyword(id)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 删除失败：%v", err))
		tb.bot.Send(msg)
		return
	}

	// 重新加载关键词
	tb.reloadKeywords()

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("✅ 关键词 ID %d 已删除", id))
	tb.bot.Send(msg)
}

func (tb *TelegramBot) handleShowViolations(chatID int64, args string) {
	limit := 10
	if args != "" {
		if l, err := strconv.Atoi(args); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	violations, err := tb.db.GetViolations(limit)
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 获取违规记录失败：%v", err))
		tb.bot.Send(msg)
		return
	}

	if len(violations) == 0 {
		msg := tgbotapi.NewMessage(chatID, "📋 暂无违规记录")
		tb.bot.Send(msg)
		return
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf("📋 最近 %d 条违规记录：\n\n", len(violations)))

	for _, v := range violations {
		text.WriteString(fmt.Sprintf("用户: %s (ID: %d)\n", v.Username, v.UserID))
		text.WriteString(fmt.Sprintf("消息: %s\n", v.MessageText))
		text.WriteString(fmt.Sprintf("触发关键词: %s\n", v.Keyword))
		text.WriteString(fmt.Sprintf("处理动作: %s\n", v.Action))
		text.WriteString(fmt.Sprintf("时间: %s\n", v.CreatedAt.Format("2006-01-02 15:04:05")))
		text.WriteString("─────────────\n")
	}

	msg := tgbotapi.NewMessage(chatID, text.String())
	tb.bot.Send(msg)
}

func (tb *TelegramBot) handleReload(chatID int64) {
	err := tb.reloadKeywords()
	if err != nil {
		msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("❌ 重新加载失败：%v", err))
		tb.bot.Send(msg)
		return
	}

	msg := tgbotapi.NewMessage(chatID, "✅ 关键词已重新加载")
	tb.bot.Send(msg)
}

func (tb *TelegramBot) handleStatus(chatID int64) {
	keywords, _ := tb.db.GetKeywords()
	violations, _ := tb.db.GetViolations(1)

	var lastViolation string
	if len(violations) > 0 {
		lastViolation = violations[0].CreatedAt.Format("2006-01-02 15:04:05")
	} else {
		lastViolation = "无"
	}

	statusText := fmt.Sprintf(`🤖 机器人状态

✅ 运行正常
📝 关键词数量: %d
⚠️ 最近违规: %s
🔧 默认动作: %s
⏰ 禁言时长: %d秒`,
		len(keywords),
		lastViolation,
		tb.config.Settings.DefaultAction,
		tb.config.Settings.MuteDuration)

	msg := tgbotapi.NewMessage(chatID, statusText)
	tb.bot.Send(msg)
}

func (tb *TelegramBot) reloadKeywords() error {
	keywords, err := tb.db.GetKeywords()
	if err != nil {
		return err
	}

	tb.filter.UpdateKeywords(keywords)
	return nil
}

func (tb *TelegramBot) checkMessageContent(message *tgbotapi.Message) {
	var textToCheck string
	var fileName string

	// 获取要检查的文本
	if message.Text != "" {
		textToCheck = message.Text
	} else if message.Caption != "" {
		textToCheck = message.Caption
	}

	// 获取文件名（如果有）
	if message.Photo != nil && len(message.Photo) > 0 {
		// 图片通常没有文件名，但检查caption
		fileName = "image"
	} else if message.Document != nil {
		fileName = message.Document.FileName
	} else if message.Video != nil {
		fileName = message.Video.FileName
	} else if message.Animation != nil {
		fileName = message.Animation.FileName
	}

	// 检查文本内容
	var result *FilterResult
	if textToCheck != "" {
		result = tb.filter.CheckMessage(textToCheck)
	}

	// 如果文本没有违规，检查文件名
	if (result == nil || !result.IsViolation) && fileName != "" {
		result = tb.filter.CheckFileName(fileName)
	}

	// 处理违规
	if result != nil && result.IsViolation {
		tb.handleViolation(message, result, textToCheck)
	}
}

func (tb *TelegramBot) handleViolation(message *tgbotapi.Message, result *FilterResult, messageText string) {
	userID := message.From.ID
	username := message.From.UserName
	if username == "" {
		username = message.From.FirstName
	}
	chatID := message.Chat.ID

	// 记录违规
	if tb.config.Settings.LogViolations {
		err := tb.db.LogViolation(userID, username, chatID, messageText, result.Keyword, result.Action)
		if err != nil {
			log.Printf("记录违规失败：%v", err)
		}
	}

	// 删除违规消息
	deleteMsg := tgbotapi.NewDeleteMessage(chatID, message.MessageID)
	tb.bot.Send(deleteMsg)

	// 执行用户处理动作
	switch result.Action {
	case "mute":
		tb.muteUser(chatID, userID)
		log.Printf("用户 %s (ID: %d) 因关键词 '%s' 被禁言", username, userID, result.Keyword)
	case "kick":
		tb.kickUser(chatID, userID)
		log.Printf("用户 %s (ID: %d) 因关键词 '%s' 被踢出", username, userID, result.Keyword)
	}

	// 发送通知给管理员
	if tb.config.Telegram.AdminUserID != 0 {
		notificationText := fmt.Sprintf(`🚨 违规检测

用户: %s (ID: %d)
群组: %s (ID: %d)
触发关键词: %s (%s匹配)
执行动作: %s
违规内容: %s`,
			username, userID,
			message.Chat.Title, chatID,
			result.Keyword, result.MatchType,
			result.Action,
			messageText)

		notifyMsg := tgbotapi.NewMessage(tb.config.Telegram.AdminUserID, notificationText)
		tb.bot.Send(notifyMsg)
	}
}

func (tb *TelegramBot) muteUser(chatID, userID int64) {
	until := time.Now().Add(time.Duration(tb.config.Settings.MuteDuration) * time.Second)

	restrictConfig := tgbotapi.ChatMemberConfig{
		ChatID: chatID,
		UserID: userID,
	}

	restrictChatMember := tgbotapi.RestrictChatMemberConfig{
		ChatMemberConfig: restrictConfig,
		UntilDate:        until.Unix(),
		Permissions: &tgbotapi.ChatPermissions{
			CanSendMessages:       false,
			CanSendMediaMessages:  false,
			CanSendPolls:          false,
			CanSendOtherMessages:  false,
			CanAddWebPagePreviews: false,
			CanChangeInfo:         false,
			CanInviteUsers:        false,
			CanPinMessages:        false,
		},
	}

	_, err := tb.bot.Request(restrictChatMember)
	if err != nil {
		log.Printf("禁言用户失败：%v", err)
	}
}

func (tb *TelegramBot) kickUser(chatID, userID int64) {
	kickConfig := tgbotapi.KickChatMemberConfig{
		ChatMemberConfig: tgbotapi.ChatMemberConfig{
			ChatID: chatID,
			UserID: userID,
		},
	}

	_, err := tb.bot.Request(kickConfig)
	if err != nil {
		log.Printf("踢出用户失败：%v", err)
	}
}
