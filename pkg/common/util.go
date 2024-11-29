package common

import (
	"github.com/google/uuid"
	"math/rand"
	"time"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-"

// 生成一个指定长度的随机字符串
func GenerateName(length int) string {
	rand.Seed(time.Now().UnixNano()) // 使用当前时间生成随机种子

	// 创建一个字符切片来存放生成的随机字符
	clientID := make([]byte, length)

	// 从charset中随机选择字符
	for i := 0; i < length; i++ {
		clientID[i] = charset[rand.Intn(len(charset))]
	}

	return string(clientID)
}

func GenerateUUID() string {
	// 生成一个UUID
	id := uuid.New()
	// 截取前8个字符来作为简短的UUID
	return id.String()[:8]
}

func Generate32ID() uint32 {
	// 使用当前时间戳作为随机数种子
	rand.Seed(time.Now().UnixNano())
	// 生成一个 [0, 2^32) 范围内的随机数
	return rand.Uint32()
}
