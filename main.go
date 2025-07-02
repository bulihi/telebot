package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func testNetwork() {
	fmt.Println("🔍 开始网络诊断...")

	// 测试DNS解析
	fmt.Println("\n1. 测试DNS解析:")
	addresses, err := net.LookupHost("api.telegram.org")
	if err != nil {
		fmt.Printf("❌ DNS解析失败: %v\n", err)
	} else {
		fmt.Printf("✅ DNS解析成功: %v\n", addresses)
	}

	// 测试HTTP连接
	fmt.Println("\n2. 测试HTTP连接:")
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get("https://api.telegram.org")
	if err != nil {
		fmt.Printf("❌ HTTP连接失败: %v\n", err)
	} else {
		fmt.Printf("✅ HTTP连接成功: 状态码 %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	// 测试其他网站连接
	fmt.Println("\n3. 测试其他网站连接:")
	resp, err = client.Get("https://www.google.com")
	if err != nil {
		fmt.Printf("❌ Google连接失败: %v\n", err)
	} else {
		fmt.Printf("✅ Google连接成功: 状态码 %d\n", resp.StatusCode)
		resp.Body.Close()
	}

	fmt.Println("\n📋 建议解决方案:")
	fmt.Println("1. 如果DNS解析失败，可能需要:")
	fmt.Println("   - 更换DNS服务器（如8.8.8.8或1.1.1.1）")
	fmt.Println("   - 检查网络防火墙设置")
	fmt.Println("2. 如果HTTP连接失败，可能需要:")
	fmt.Println("   - 使用代理服务器")
	fmt.Println("   - 检查是否有网络限制")
	fmt.Println("3. 在代码中启用代理的方法:")
	fmt.Println("   修改 bot.go 中的代理设置，取消注释并填入你的代理地址")
}

func main() {
	// 检查是否运行网络诊断
	if len(os.Args) > 1 && os.Args[1] == "test-network" {
		testNetwork()
		return
	}

	// 加载配置
	config, err := LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("加载配置失败：%v", err)
	}

	// 验证配置
	err = config.Validate()
	if err != nil {
		log.Fatalf("配置验证失败：%v", err)
	}

	// 初始化数据库
	db, err := NewDatabase(config.Database.Path)
	if err != nil {
		log.Fatalf("初始化数据库失败：%v", err)
	}
	defer db.Close()

	// 执行数据库迁移
	log.Println("正在检查数据库结构...")
	if err := db.Migrate(); err != nil {
		log.Fatalf("数据库迁移失败：%v", err)
	}

	// 创建重载通道
	reloadChan := make(chan struct{}, 1)

	// 创建机器人
	bot, err := NewTelegramBot(config, db)
	if err != nil {
		log.Fatalf("创建机器人失败：%v", err)
	}

	// 启动Web管理界面
	go func() {
		webServer := NewWebServer(config, db, reloadChan, bot.bot)
		log.Printf("Web管理界面启动在端口 %s", config.Server.Port)
		err := webServer.Start()
		if err != nil {
			log.Printf("Web服务器启动失败：%v", err)
		}
	}()

	// 处理重载信号
	go func() {
		for range reloadChan {
			log.Println("收到重载信号，正在重新加载关键词...")
			keywords, err := db.GetKeywords()
			if err != nil {
				log.Printf("重新加载关键词失败：%v", err)
				continue
			}
			bot.filter.UpdateKeywords(keywords)
			log.Printf("关键词重新加载完成，共 %d 个关键词", len(keywords))
		}
	}()

	// 设置优雅关闭
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		log.Println("正在关闭机器人...")
		close(reloadChan)
		os.Exit(0)
	}()

	log.Println("机器人已启动，按 Ctrl+C 停止")

	// 启动机器人
	bot.Start()
}
