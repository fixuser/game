package models

import (
	"crypto/rand"

	"golang.org/x/crypto/argon2"
)

type Argon2Data struct {
	Version    uint8
	Iterations uint32 // 迭代次数
	Memory     uint32 // KB
	Threads    uint8
	KeyLength  uint32
	SaltLen    uint8
	Salt       []byte
	Password   []byte
}

// Default 设置默认参数
func (a *Argon2Data) Default() {
	a.Version = argon2.Version
	a.Memory = 64 * 1024
	a.Iterations = 3
	a.Threads = 2
	a.SaltLen = 16
	a.KeyLength = 32
	a.Salt = make([]byte, a.SaltLen)
	rand.Reader.Read(a.Salt)
}

// Check 检查密码是否匹配
func (a *Argon2Data) Check(password string) bool {
	hashed := argon2.IDKey([]byte(password), a.Salt, a.Iterations, a.Memory, a.Threads, a.KeyLength)
	return string(hashed) == string(a.Password)
}

// Generate 生成哈希密码
func (a *Argon2Data) Generate(password string) {
	a.Default()
	hashed := argon2.IDKey([]byte(password), a.Salt, a.Iterations, a.Memory, a.Threads, a.KeyLength)
	a.Password = hashed
}
