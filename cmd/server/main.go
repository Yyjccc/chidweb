package main

import (
	"chidweb/pkg/common"
	"chidweb/pkg/server"
	"flag"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"strings"
)

func main() {

	tcpPort := flag.String("p", "0", "tcp listener port ")
	httpPort := flag.Int("s", 8080, "http listener port")

	//指定配置文件
	c2FilePath := flag.String("cf", "", "config file path")

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
	}
	ports := strings.Split(*tcpPort, ",")

	config := common.DefaultConfig
	if *c2FilePath != "" {
		config = common.NewConfigByFile(*c2FilePath)
	} else {
		common.Info("no detect config file ,use default config!")
		config = common.DefaultConfig
	}
	ser := server.NewServer(config, *httpPort, ports)
	err = ser.Start()
	if err != nil {
		common.Error("%v", err)
	}
	ser.Wait()

}
