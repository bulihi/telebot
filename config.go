package main

import (
	"io/ioutil"
	"log"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Telegram struct {
		BotToken    string `yaml:"bot_token"`
		AdminUserID int64  `yaml:"admin_user_id"`
	} `yaml:"telegram"`
	Database struct {
		Path string `yaml:"path"`
	} `yaml:"database"`
	Server struct {
		Port          string `yaml:"port"`
		AdminPassword string `yaml:"admin_password"`
	} `yaml:"server"`
	Proxy struct {
		Enabled bool   `yaml:"enabled"`
		URL     string `yaml:"url"`
	} `yaml:"proxy"`
	Settings struct {
		DefaultAction string `yaml:"default_action"`
		MuteDuration  int    `yaml:"mute_duration"`
		LogViolations bool   `yaml:"log_violations"`
	} `yaml:"settings"`
	Groups struct {
		DefaultSettings struct {
			WelcomeMessage string `yaml:"welcome_message"`
			Verification   struct {
				Enabled  bool   `yaml:"enabled"`
				Question string `yaml:"question"`
				Answer   string `yaml:"answer"`
				Timeout  int    `yaml:"timeout"`
			} `yaml:"verification"`
		} `yaml:"default_settings"`
	} `yaml:"groups"`
}

func LoadConfig(filename string) (*Config, error) {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return nil, err
	}

	return &config, nil
}

func (c *Config) Validate() error {
	if c.Telegram.BotToken == "" || c.Telegram.BotToken == "YOUR_BOT_TOKEN_HERE" {
		log.Fatal("请在 config.yaml 中设置正确的 bot_token")
	}

	if c.Telegram.AdminUserID == 0 {
		log.Fatal("请在 config.yaml 中设置管理员用户ID")
	}

	if c.Server.AdminPassword == "" || c.Server.AdminPassword == "your_admin_password_here" {
		log.Fatal("请在 config.yaml 中设置管理页面密码")
	}

	return nil
}
