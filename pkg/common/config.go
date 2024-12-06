package common

import (
	"container/list"
	"embed"
	"gopkg.in/yaml.v3"
	"io/fs"
	"io/ioutil"
	"strings"
	"time"
)

//go:embed config.yaml
var configFile embed.FS

//go:embed banner.txt
var Banner string

const Version = "v0.0.1"

var (
	InitConfig *BasicConfig
	//开启流量混淆默认配置
	DefaultConfig *BasicConfig
)

func init() {
	InitConfig = &BasicConfig{
		Rate:        3 * time.Second,
		EnableProxy: false,
		Obfuscation: true,
	}

	//读取绑定的配置文件
	data, err := fs.ReadFile(configFile, "config.yaml")
	if err != nil {
		panic(err)
	}
	var config BasicConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		panic(err)
	}
	DefaultConfig = ParseConfig(&config)
}

// 基本配置
type BasicConfig struct {
	//心跳速率
	RateConfig  int           `yaml:"rate"`
	Rate        time.Duration `yaml:"-"`
	EnableProxy bool          `yaml:"enableProxy"` //是否启动代理
	Proxies     []string      `yaml:"proxies"`     //代理池
	Obfuscation bool
	LogLevel    string          `yaml:"log_level"` //	日志级别
	LoggerFile  string          `yaml:"log_file"`  //输出日志文件
	Custom      []*CustomConfig `yaml:"custom"`
	Key         *KeyConfig      `yaml:"key"`
}

type CustomConfig struct {
	Name           string `yaml:"name"`
	Description    string `yaml:"description"`
	Path           string `yaml:"path"`
	Probability    uint32 `yaml:"probability"` //概率
	ResponseConfig `yaml:"response"`
	RequestConfig  `yaml:"request"`
}

type RequestConfig struct {
	Headers        []string   `yaml:"headers"`
	BeforeAppend   string     `yaml:"before_append"`
	Method         string     `yaml:"method"`
	AfterAppend    string     `yaml:"after_append"`
	TransformChain string     `yaml:"transformers"`
	Transformers   *list.List `yaml:"-"`
}

type ResponseConfig struct {
	Codes          []int      `yaml:"codes"`
	Headers        []string   `yaml:"headers"`
	BeforeAppend   string     `yaml:"before_append"`
	AfterAppend    string     `yaml:"after_append"`
	TransformChain string     `yaml:"transformers"`
	Transformers   *list.List `yaml:"-"`
}

type KeyConfig struct {
	AesKey string `yaml:"aes"`
	Rc4Key string `yaml:"rc4"`
	XorKey string `yaml:"xor"`
	Image  string `yaml:"image"`
}

func NewConfigByFile(filepath string) *BasicConfig {
	data, err := ioutil.ReadFile(filepath)
	if err != nil {
		panic("not found file :" + filepath)

	}
	var config BasicConfig
	err = yaml.Unmarshal(data, &config)
	if err != nil {
		panic(err)
	}
	return ParseConfig(&config)
}

func NewConfig(rate int, enable bool, proxies []string, obfuscation bool, logLevel string, loggerFile string) *BasicConfig {
	config := &BasicConfig{
		Rate:        time.Duration(rate) * time.Second,
		EnableProxy: enable,
		Proxies:     proxies,
		Obfuscation: obfuscation,
		LogLevel:    logLevel,
		LoggerFile:  loggerFile,
		Custom:      nil,
	}
	return config
}

func ParseConfig(config *BasicConfig) *BasicConfig {
	config.Rate = time.Duration(config.RateConfig) * time.Second
	for _, custom := range config.Custom {
		if custom.RequestConfig.TransformChain != "" {
			l := list.New()
			splits := strings.Split(custom.RequestConfig.TransformChain, "->")
			for _, split := range splits {
				var transformer Transformer
				switch TransformType(split) {
				case AesTransform:
					transformer = NewAESTransformer(config.Key.AesKey)
					break
				case Base64Transform:
					transformer = NewBase64Transformer()
					break
				case HexTransform:
					transformer = NewHexTransformer()
					break
				case AppendTransform:
					transformer = NewAppendTransformer(custom.RequestConfig.BeforeAppend, custom.RequestConfig.AfterAppend)
					break
				case XorTransform:
					transformer = NewXORTransformer(config.Key.XorKey)
					break
				case RC4Transform:
					transformer = NewRC4Transformer(config.Key.Rc4Key)
					break
				case ImageTransform:
					transformer = NewImageTransformer(config.Key.Image)
					break
				default:
					panic("unknown transform type :" + split)
				}
				l.PushBack(transformer)
			}
			custom.RequestConfig.Transformers = l
		}
		if custom.ResponseConfig.TransformChain != "" {
			l := list.New()
			splits := strings.Split(custom.ResponseConfig.TransformChain, "->")
			for _, split := range splits {
				var transformer Transformer
				switch TransformType(split) {
				case AesTransform:
					transformer = NewAESTransformer(config.Key.AesKey)
					break
				case Base64Transform:
					transformer = NewBase64Transformer()
					break
				case HexTransform:
					transformer = NewHexTransformer()
					break
				case AppendTransform:
					transformer = NewAppendTransformer(custom.ResponseConfig.BeforeAppend, custom.ResponseConfig.AfterAppend)
					break
				case XorTransform:
					transformer = NewXORTransformer(config.Key.XorKey)
					break
				case RC4Transform:
					transformer = NewRC4Transformer(config.Key.Rc4Key)
					break
				case ImageTransform:
					transformer = NewImageTransformer(config.Key.Image)
					break
				default:
					panic("unknown transform type :" + split)
				}
				l.PushBack(transformer)
			}
			custom.ResponseConfig.Transformers = l
		}

	}
	return config
}
