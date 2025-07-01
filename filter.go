package main

import (
	"net/url"
	"regexp"
	"strings"
)

type MessageFilter struct {
	keywords []Keyword
}

type FilterResult struct {
	IsViolation bool
	Keyword     string
	Action      string
	MatchType   string
}

func NewMessageFilter(keywords []Keyword) *MessageFilter {
	return &MessageFilter{
		keywords: keywords,
	}
}

func (f *MessageFilter) UpdateKeywords(keywords []Keyword) {
	f.keywords = keywords
}

func (f *MessageFilter) CheckMessage(messageText string) *FilterResult {
	// 检查文本消息
	if result := f.checkTextMessage(messageText); result.IsViolation {
		return result
	}

	// 检查链接
	if result := f.checkLinks(messageText); result.IsViolation {
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
	// 检查HTTP/HTTPS链接
	urlRegex := regexp.MustCompile(`https?://[^\s]+`)
	urls := urlRegex.FindAllString(text, -1)

	for _, urlStr := range urls {
		for _, keyword := range f.keywords {
			keywordLower := strings.ToLower(keyword.Keyword)
			urlLower := strings.ToLower(urlStr)

			// 解析URL
			if parsedURL, err := url.Parse(urlStr); err == nil {
				domainLower := strings.ToLower(parsedURL.Host)
				pathLower := strings.ToLower(parsedURL.Path)

				switch keyword.MatchType {
				case "exact":
					if f.exactMatch(urlLower, keywordLower) ||
						f.exactMatch(domainLower, keywordLower) ||
						f.exactMatch(pathLower, keywordLower) {
						return &FilterResult{
							IsViolation: true,
							Keyword:     keyword.Keyword,
							Action:      keyword.Action,
							MatchType:   keyword.MatchType,
						}
					}
				case "fuzzy":
					if f.fuzzyMatch(urlLower, keywordLower) ||
						f.fuzzyMatch(domainLower, keywordLower) ||
						f.fuzzyMatch(pathLower, keywordLower) {
						return &FilterResult{
							IsViolation: true,
							Keyword:     keyword.Keyword,
							Action:      keyword.Action,
							MatchType:   keyword.MatchType,
						}
					}
				case "regex":
					if f.regexMatch(urlStr, keyword.Keyword) ||
						f.regexMatch(parsedURL.Host, keyword.Keyword) ||
						f.regexMatch(parsedURL.Path, keyword.Keyword) {
						return &FilterResult{
							IsViolation: true,
							Keyword:     keyword.Keyword,
							Action:      keyword.Action,
							MatchType:   keyword.MatchType,
						}
					}
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
