package main

import (
	client2 "chidweb/pkg/client"
	"chidweb/pkg/common"
	"flag"
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	//must
	serverAddr := flag.String("s", "", "server url")
	targetAddr := flag.String("t", "", "target tcp address,split:','")

	//Optional
	rate := flag.Int("i", 3, "heartbeat packet rate (Unit:s)")
	enableProxy := flag.Bool("enable-proxy", false, "enable proxy")
	proxyStr := flag.String("p", "", "proxy server urls,split:','")
	//指定配置文件
	c2FilePath := flag.String("cf", "", "config file path")
	//关闭流量混淆
	Obfuscation := flag.Bool("ob", true, "use obfuscation")

	// 日志相关参数
	logLevel := flag.String("ll", "info", "log level (panic|fatal|error|warn|info|debug|trace)")
	loggerFilePath := flag.String("lf", "", "log file path (empty for stdout)")
	noLog := flag.Bool("ln", false, "disable all logging")
	flag.Parse()

	// 验证日志级别
	validLevels := []string{"panic", "fatal", "error", "warn", "info", "debug", "trace"}
	isValidLevel := false
	for _, valid := range validLevels {
		if valid == strings.ToLower(*logLevel) {
			isValidLevel = true
			break
		}
	}
	if !isValidLevel {
		common.Error("Invalid log level: %s. Valid levels are: %s", *logLevel, strings.Join(validLevels, ", "))
		os.Exit(1)
	}

	// 设置日志级别
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		common.Error("Failed to parse log level: %v", err)
		os.Exit(1)
	}
	common.SetLevel(level)

	// 设置日志输出
	if *noLog {
		common.DisableLogging()
	} else if *loggerFilePath != "" {
		// 验证日志文件路径
		dir := filepath.Dir(*loggerFilePath)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			common.Error("Log file directory does not exist: %s", dir)
			os.Exit(1)
		}
		if err := common.SetOutput(*loggerFilePath); err != nil {
			common.Error("Failed to set log file: %v", err)
			os.Exit(1)
		}
		common.Info("Log file set to: %s", *loggerFilePath)
	} else {
		fmt.Println(common.Banner)
	}

	var config *common.BasicConfig

	if *serverAddr == "" {
		common.Error("server url is required.")
		os.Exit(1)
	}
	if *targetAddr == "" {
		common.Error("target tcp address is required.")
		os.Exit(1)
	}

	targets := strings.Split(*targetAddr, ",")
	proxies := strings.Split(*proxyStr, ",")
	clientID := common.GenerateName(7)
	if *c2FilePath != "" {
		config = common.NewConfigByFile(*c2FilePath)
	} else {
		if *Obfuscation {
			common.Info("detect not hava config file ,use default config for obfuscation")
			config = common.DefaultConfig
		} else {
			common.Warn("not use obfuscation or custom config is dangerous! suggest you use one of these.")
			config = common.NewConfig(*rate, *enableProxy, proxies, *Obfuscation, *logLevel, *loggerFilePath)
		}

	}

	cli := client2.NewClient(*serverAddr, clientID, targets, config)

	if err := cli.Start(); err != nil {
		common.Fatal(err.Error())
	}

	cli.Wait()
}
