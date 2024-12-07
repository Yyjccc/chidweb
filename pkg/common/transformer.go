package common

import (
	"bytes"
	"container/list"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rc4"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

type TransformType string

const (
	ImageTransform  TransformType = "image"
	XorTransform    TransformType = "xor"
	Base64Transform TransformType = "base64"
	AesTransform    TransformType = "aes"
	RC4Transform    TransformType = "rc4"
	HexTransform    TransformType = "hex"
	AppendTransform TransformType = "append"
)

// 流量 处理
type TransformerHandler struct {
	config               *BasicConfig
	requestTransformers  *list.List
	responseTransformers *list.List
}

func NewTransformerHandler(config *BasicConfig) *TransformerHandler {
	return &TransformerHandler{
		config: config,
	}
}

type Transformer interface {
	Transform(data []byte) []byte
	UnTransform(data []byte) ([]byte, error)
}

// RC4Transformer RC4加密变换器
type RC4Transformer struct {
	key []byte
}

func NewRC4Transformer(key string) *RC4Transformer {
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		panic(err)
	}
	return &RC4Transformer{key: keyBytes}
}

func (t *RC4Transformer) Transform(data []byte) []byte {
	cip, _ := rc4.NewCipher(t.key)
	result := make([]byte, len(data))
	cip.XORKeyStream(result, data)
	return result
}

func (t *RC4Transformer) UnTransform(data []byte) ([]byte, error) {
	return t.Transform(data), nil // RC4 解密与加密操作相同
}

// AESTransformer AES加密变换器
type AESTransformer struct {
	key []byte
}

func NewAESTransformer(key string) *AESTransformer {

	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		panic(err)
	}
	return &AESTransformer{key: keyBytes}
}

// Transform 使用随机 IV 加密数据
func (t *AESTransformer) Transform(data []byte) []byte {

	defer func() {
		if err := recover(); err != nil {
			Error("[transformer] panic: ", err)
		}
	}()
	block, err := aes.NewCipher(t.key)
	if err != nil {
		Error("[transformer] failed to create AES cipher: %w", err)
		return nil
	}

	// 生成随机 IV
	iv := make([]byte, block.BlockSize())
	if _, err := rand.Read(iv); err != nil {
		Error("[transformer] failed to generate IV: %w", err)
		return nil
	}

	// PKCS7 填充数据
	data = PKCS7Padding(data, block.BlockSize())

	// 使用 CBC 模式加密
	blockMode := cipher.NewCBCEncrypter(block, iv)
	crypted := make([]byte, len(data))
	blockMode.CryptBlocks(crypted, data)

	// 将 IV 和加密数据合并返回
	result := append(iv, crypted...)
	//Debug("AES 输出：%s", base64.StdEncoding.EncodeToString(result))
	return result
}

// UnTransform 解密数据（从密文中提取 IV）
func (t *AESTransformer) UnTransform(data []byte) ([]byte, error) {
	//Debug("AES 输入：%s", base64.StdEncoding.EncodeToString(data))
	block, err := aes.NewCipher(t.key)
	if err != nil {
		return nil, fmt.Errorf("[transformer] failed to create AES cipher: %w", err)
	}

	blockSize := block.BlockSize()
	if len(data) < blockSize {
		return nil, errors.New("[transformer] aes invalid data length")
	}

	// 提取 IV 和实际密文
	iv := data[:blockSize]
	crypted := data[blockSize:]

	// 使用 CBC 模式解密
	blockMode := cipher.NewCBCDecrypter(block, iv)
	origData := make([]byte, len(crypted))
	blockMode.CryptBlocks(origData, crypted)

	// 去除 PKCS7 填充
	origData, err = PKCS7UnPadding(origData)
	if err != nil {
		return nil, fmt.Errorf("[transformer] aes failed to unpad data: %w", err)
	}

	return origData, nil
}

// PKCS7Padding 实现 PKCS7 填充
func PKCS7Padding(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padText := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padText...)
}

// PKCS7UnPadding 移除 PKCS7 填充
func PKCS7UnPadding(data []byte) ([]byte, error) {
	length := len(data)
	if length == 0 {
		return nil, errors.New("[transformer] aes data is empty")
	}
	padding := int(data[length-1])
	if padding > length || padding == 0 {
		return nil, errors.New("[transformer] aesinvalid padding")
	}
	return data[:length-padding], nil
}

// Base64Transformer Base64编码变换器
type Base64Transformer struct{}

func NewBase64Transformer() *Base64Transformer {
	return &Base64Transformer{}
}

func (t *Base64Transformer) Transform(data []byte) []byte {
	// 使用 Base64 URL 编码
	encoded := base64.URLEncoding.EncodeToString(data)
	//Debug("base64 结果：%s", encoded)
	return []byte(encoded)
}

func (t *Base64Transformer) UnTransform(data []byte) ([]byte, error) {
	//Debug("base64 输入：%s", string(data))
	return base64.URLEncoding.DecodeString(string(data))
}

// HexTransformer 十六进制编码变换器
type HexTransformer struct{}

func NewHexTransformer() *HexTransformer {
	return &HexTransformer{}
}

func (t *HexTransformer) Transform(data []byte) []byte {
	return []byte(hex.EncodeToString(data))
}

func (t *HexTransformer) UnTransform(data []byte) ([]byte, error) {
	return hex.DecodeString(string(data))
}

type AppendTransformer struct {
	beforeAppend []byte
	afterAppend  []byte
}

func NewAppendTransformer(before, after string) *AppendTransformer {
	return &AppendTransformer{
		beforeAppend: []byte(before),
		afterAppend:  []byte(after),
	}
}

// Transform 在数据前后追加字节
func (a AppendTransformer) Transform(data []byte) []byte {

	//Debug("append 前数据：%s", string(data))
	// 提前计算所需总长度以减少内存分配
	totalLength := len(a.beforeAppend) + len(data) + len(a.afterAppend)
	transformed := make([]byte, totalLength)

	// 将字节依次拷贝到目标切片中
	copy(transformed, a.beforeAppend)                                // 前缀
	copy(transformed[len(a.beforeAppend):], data)                    // 数据主体
	copy(transformed[len(a.beforeAppend)+len(data):], a.afterAppend) // 后缀
	//Debug("append 后数据：%s", string(transformed))
	return transformed
}

// UnTransform 尝试还原数据
func (a AppendTransformer) UnTransform(data []byte) ([]byte, error) {
	// 检查数据长度是否足够
	if len(data) < len(a.beforeAppend)+len(a.afterAppend) {
		return nil, errors.New("[transformer] append data too short to untransform")
	}

	// 验证前缀
	if !bytes.HasPrefix(data, a.beforeAppend) {
		return nil, errors.New("[transformer] append data does not have expected prefix")
	}

	// 验证后缀
	if !bytes.HasSuffix(data, a.afterAppend) {
		return nil, errors.New("[transformer] append data does not have expected suffix")
	}

	// 提取中间部分
	start := len(a.beforeAppend)
	end := len(data) - len(a.afterAppend)
	return data[start:end], nil
}

type XORTransformer struct {
	key []byte
}

func NewXORTransformer(key string) *XORTransformer {
	keyBytes, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		panic(err)
	}
	return &XORTransformer{
		key: keyBytes,
	}
}

func (t *XORTransformer) Transform(data []byte) []byte {
	result := make([]byte, len(data))
	for i := 0; i < len(data); i++ {
		result[i] = data[i] ^ t.key[i%len(t.key)]
	}
	return result
}

func (t *XORTransformer) UnTransform(data []byte) ([]byte, error) {
	return t.Transform(data), nil
}

// ImageTransformer 用于图像数据的处理
type ImageTransformer struct {
	// 图片数据以 base64 编码存储在结构体中
	BaseImgData []byte
	endPos      int
}

// NewImageTransformer 创建 ImageTransformer 实例
func NewImageTransformer(imgDataBase64 string) *ImageTransformer {
	pngData, err := base64.StdEncoding.DecodeString(imgDataBase64)
	if err != nil {
		panic(fmt.Errorf("[transformer] base64解码PNG数据失败: %v", err))
	}

	return &ImageTransformer{
		BaseImgData: pngData,
	}
}

// Transform 将数据以 `tEXt` 块的形式追加到PNG数据
func (t *ImageTransformer) Transform(data []byte) []byte {
	// 输出调试信息
	//Debug("image 写入：%s", base64.URLEncoding.EncodeToString(data))

	pngData := t.BaseImgData
	newPngData := append(pngData, data...)
	//Debug("image 输出：%s", base64.URLEncoding.EncodeToString(newPngData))
	return newPngData
}

// UnTransform 从PNG数据中提取 `tEXt` 块中的数据
func (t *ImageTransformer) UnTransform(srcImg []byte) ([]byte, error) {
	//Debug("输入 image：%s", base64.URLEncoding.EncodeToString(srcImg))
	if len(srcImg) < 10 {
		return nil, fmt.Errorf("[transformer] 图像数据太小，无法提取数据")
	}

	// 提取最后10个字节的附加数据
	additionalData := srcImg[len(t.BaseImgData):]
	//Debug("image 读出：%s", base64.URLEncoding.EncodeToString(additionalData))
	return additionalData, nil
}
