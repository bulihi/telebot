package main

import (
	"net/url"
	"regexp"
	"strings"
)

type MessageFilter struct {
	keywords []Keyword
	// 预编译的正则表达式
	adPatterns []*regexp.Regexp
}

type FilterResult struct {
	IsViolation bool
	Keyword     string
	Action      string
	MatchType   string
}

func NewMessageFilter(keywords []Keyword) *MessageFilter {
	// 定义广告特征模式
	patterns := []string{
		`(?i)广[告告]待?[发發]`, // 广告待发/广告发
		`(?i)精准.*?[群裙]`,   // 精准xxx群
		`(?i)私信.*?[拉私]人`,  // 私信拉人
		`(?i)强拉`,          // 强拉
		`(?i)引流`,          // 引流
		`(?i)代开会员`,        // 代开会员
		`(?i)包效果`,         // 包效果
		`(?i)双向`,          // 双向
		`(?i)全行业`,         // 全行业
		`(?i)实时查看`,        // 实时查看
		`(?i)分类群`,         // 分类群
		`(?i)活粉`,          // 活粉
		`(?i)\d+万.*?[群裙]`, // 数字万xxx群
		`(?i)代发`,          // 代发
	}

	// 预编译正则表达式
	var compiledPatterns []*regexp.Regexp
	for _, p := range patterns {
		if r, err := regexp.Compile(p); err == nil {
			compiledPatterns = append(compiledPatterns, r)
		}
	}

	return &MessageFilter{
		keywords:   keywords,
		adPatterns: compiledPatterns,
	}
}

func (f *MessageFilter) UpdateKeywords(keywords []Keyword) {
	f.keywords = keywords
}

func (f *MessageFilter) CheckMessage(messageText string) *FilterResult {
	// 1. 检查是否包含广告特征
	hasAdPattern := false
	for _, pattern := range f.adPatterns {
		if pattern.MatchString(messageText) {
			hasAdPattern = true
			break
		}
	}

	// 2. 提取所有用户名
	usernameRegex := regexp.MustCompile(`@[a-zA-Z0-9_]+`)
	usernames := usernameRegex.FindAllString(messageText, -1)

	// 3. 如果消息包含广告特征和被禁用的用户名，直接禁止
	if hasAdPattern && len(usernames) > 0 {
		for _, username := range usernames {
			cleanUsername := strings.TrimPrefix(username, "@")
			for _, keyword := range f.keywords {
				cleanKeyword := strings.TrimPrefix(keyword.Keyword, "@")
				if strings.EqualFold(cleanUsername, cleanKeyword) {
					return &FilterResult{
						IsViolation: true,
						Keyword:     keyword.Keyword,
						Action:      keyword.Action,
						MatchType:   "ad_with_username",
					}
				}
			}
		}
	}

	// 4. 检查关键词匹配
	if result := f.checkTextMessage(messageText); result.IsViolation {
		return result
	}

	// 5. 检查链接
	if result := f.checkLinks(messageText); result.IsViolation {
		return result
	}

	// 6. 检查用户名
	if result := f.checkUsernames(messageText); result.IsViolation {
		return result
	}

	return &FilterResult{IsViolation: false}
}

func (f *MessageFilter) checkTextMessage(text string) *FilterResult {
	textLower := strings.ToLower(text)

	for _, keyword := range f.keywords {
		keywordLower := strings.ToLower(keyword.Keyword)

		switch keyword.MatchType {
		case "exact":
			if f.exactMatch(textLower, keywordLower) {
				return &FilterResult{
					IsViolation: true,
					Keyword:     keyword.Keyword,
					Action:      keyword.Action,
					MatchType:   keyword.MatchType,
				}
			}
		case "fuzzy":
			if f.fuzzyMatch(textLower, keywordLower) {
				return &FilterResult{
					IsViolation: true,
					Keyword:     keyword.Keyword,
					Action:      keyword.Action,
					MatchType:   keyword.MatchType,
				}
			}
		case "regex":
			if f.regexMatch(text, keyword.Keyword) {
				return &FilterResult{
					IsViolation: true,
					Keyword:     keyword.Keyword,
					Action:      keyword.Action,
					MatchType:   keyword.MatchType,
				}
			}
		}
	}

	return &FilterResult{IsViolation: false}
}

func (f *MessageFilter) checkLinks(text string) *FilterResult {
	// 匹配 t.me 链接
	tmeRegex := regexp.MustCompile(`(?i)t\.me/[a-zA-Z0-9_]+`)
	matches := tmeRegex.FindAllString(text, -1)

	for _, match := range matches {
		for _, keyword := range f.keywords {
			if strings.Contains(strings.ToLower(match), strings.ToLower(keyword.Keyword)) {
				return &FilterResult{
					IsViolation: true,
					Keyword:     keyword.Keyword,
					Action:      keyword.Action,
					MatchType:   "link",
				}
			}
		}
	}

	// 匹配其他链接
	urlRegex := regexp.MustCompile(`https?://[^\s]+`)
	matches = urlRegex.FindAllString(text, -1)

	for _, match := range matches {
		parsedURL, err := url.Parse(match)
		if err != nil {
			continue
		}

		for _, keyword := range f.keywords {
			if strings.Contains(strings.ToLower(parsedURL.Host), strings.ToLower(keyword.Keyword)) ||
				strings.Contains(strings.ToLower(parsedURL.Path), strings.ToLower(keyword.Keyword)) {
				return &FilterResult{
					IsViolation: true,
					Keyword:     keyword.Keyword,
					Action:      keyword.Action,
					MatchType:   "link",
				}
			}
		}
	}

	return &FilterResult{IsViolation: false}
}

func (f *MessageFilter) checkUsernames(text string) *FilterResult {
	// 匹配 @ 用户名
	usernameRegex := regexp.MustCompile(`@[a-zA-Z0-9_]+`)
	matches := usernameRegex.FindAllString(text, -1)

	for _, match := range matches {
		username := strings.TrimPrefix(match, "@")
		for _, keyword := range f.keywords {
			// 如果关键词以 @ 开头，移除它进行比较
			keywordUsername := strings.TrimPrefix(keyword.Keyword, "@")

			if strings.EqualFold(username, keywordUsername) {
				return &FilterResult{
					IsViolation: true,
					Keyword:     keyword.Keyword,
					Action:      keyword.Action,
					MatchType:   "username",
				}
			}
		}
	}

	return &FilterResult{IsViolation: false}
}

func (f *MessageFilter) exactMatch(text, keyword string) bool {
	// 检查完全匹配（作为独立单词）
	words := strings.Fields(text)
	for _, word := range words {
		// 移除标点符号
		cleanWord := regexp.MustCompile(`[^\p{L}\p{N}]+`).ReplaceAllString(word, "")
		if cleanWord == keyword {
			return true
		}
	}

	// 也检查作为子字符串的精确匹配
	return strings.Contains(text, keyword)
}

func (f *MessageFilter) fuzzyMatch(text, keyword string) bool {
	// 模糊匹配：包含关键词
	return strings.Contains(text, keyword)
}

func (f *MessageFilter) regexMatch(text, pattern string) bool {
	// 正则表达式匹配
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return regex.MatchString(text)
}

// 检查图片或文件的文件名
func (f *MessageFilter) CheckFileName(fileName string) *FilterResult {
	if fileName == "" {
		return &FilterResult{IsViolation: false}
	}

	return f.checkTextMessage(fileName)
}

// 检查图片的caption
func (f *MessageFilter) CheckCaption(caption string) *FilterResult {
	if caption == "" {
		return &FilterResult{IsViolation: false}
	}

	return f.CheckMessage(caption)
}
