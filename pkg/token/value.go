package token

import (
	json "github.com/goccy/go-json"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// TokenValue 保存 Token 的完整信息
// 实现 encoding.BinaryMarshaler / encoding.BinaryUnmarshaler，
// 可直接用于 rdb.Set(ctx, key, &tv, ttl) 和 rdb.Get(...).Scan(&tv)
type TokenValue struct {
	UserId       int64  `json:"user_id"`
	UserType     string `json:"user_type"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Platform     string `json:"platform"`
	CreatedAt    int64  `json:"created_at"`
	AccessTtl    int64  `json:"access_ttl"`
	RefreshTtl   int64  `json:"refresh_ttl"`
	Extras       []byte `json:"extras"`
}

// MarshalBinary 实现 encoding.BinaryMarshaler，内部使用 go-json 序列化
func (tv TokenValue) MarshalBinary() ([]byte, error) {
	return json.Marshal(tv)
}

// UnmarshalBinary 实现 encoding.BinaryUnmarshaler，内部使用 go-json 反序列化
func (tv *TokenValue) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, tv)
}

// SetExtra 向 Extras 中写入指定 key 的值
//
//	tv.SetExtra("role", "admin")
func (tv *TokenValue) SetExtra(key string, value any) {
	tv.Extras, _ = sjson.SetBytes(tv.Extras, key, value)
}

// GetExtra 从 Extras 中读取指定 key 的值
//
//	role := tv.GetExtra("role").String()
func (tv *TokenValue) GetExtra(key string) gjson.Result {
	return gjson.GetBytes(tv.Extras, key)
}
