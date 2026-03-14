package models

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/game/game/apis/basepb"
	"github.com/game/game/pkg/tag"
	"github.com/goccy/go-json"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/uptrace/bun"
)

// User 用户
type User struct {
	Id           int64               `bun:",pk"`
	InviteIds    []int64             `bun:",array,nullzero"`        // 邀请关系
	Type         basepb.User_Type    `bun:",notnull,type:smallint"` // 用户类型
	Nickname     string              `bun:",nullzero"`              // 昵称
	Avatar       string              `bun:",nullzero"`              // 头像
	Gender       basepb.User_Gender  `bun:",notnull,type:smallint"` // 性别
	RegType      basepb.User_RegType `bun:",notnull,type:smallint"` // 注册类型
	Email        string              `bun:",nullzero"`              // 邮箱
	AreaCode     int16               `bun:",notnull,type:smallint"` // 地区 86,1,852
	Phone        string              `bun:",nullzero"`              // 电话
	VerifiedTags tag.Tag             `bun:",nullzero,type:bigint"`  // 验证情况
	Password     *Argon2Data         `bun:",nullzero,type:jsonb"`   // 密码
	Point        decimal.Decimal     `bun:",notnull,type:decimal"`  // 积分
	TotalPoint   decimal.Decimal     `bun:",notnull,type:decimal"`  // 累计积分
	Ip           string              `bun:",nullzero"`              // 注册时候的IP地址
	DeviceId     string              `bun:",nullzero"`              // 注册时候的设备ID
	Tags         tag.Tag             `bun:",nullzero,type:bigint"`  // 标签
	Status       basepb.User_Status  `bun:",notnull,type:smallint"` // 状态
	Extras       json.RawMessage     `bun:",nullzero,type:jsonb"`   // 扩展信息
	TerminatedAt *time.Time          `bun:",nullzero"`              // 终止时间
	CreatedAt    time.Time           `bun:",nullzero,notnull,default:current_timestamp"`
	UpdatedAt    time.Time           `bun:",nullzero,notnull,default:current_timestamp"`
}

// GenUserId 生成用户ID
func GenUserId() int64 {
	return 1000000 + rand.Int64N(8999999)
}

// GenUserIds 生成用户ID列表
func GenUserIds(count int) []int64 {
	ids := make([]int64, count)
	for i := range count {
		ids[i] = GenUserId()
	}
	return ids
}

// BeforeAppendModel 在添加数据前
func (u *User) BeforeAppendModel(ctx context.Context, query bun.Query) error {
	switch query.(type) {
	case *bun.InsertQuery:
		if u.Id == 0 {
			u.Id = GenUserId()
		}
		if u.Status != basepb.User_STATUS_UNSPECIFIED {
			u.Status = basepb.User_STATUS_NORMAL
		}
		if u.Type != basepb.User_TYPE_UNSPECIFIED {
			u.Type = basepb.User_TYPE_USER
		}
	case *bun.UpdateQuery:
	}
	return nil
}

// IsNormal 是否正常
func (u *User) IsNormal() bool {
	return u.Status == basepb.User_STATUS_NORMAL
}

// IsDisabled 是否禁用
func (u *User) IsDisabled() bool {
	return u.Status == basepb.User_STATUS_DISABLED
}

// IsTerminated 是否终止
func (u *User) IsTerminated() bool {
	return u.Status == basepb.User_STATUS_TERMINATED
}

// GetInviteIds 获取邀请码
func (u *User) GetInviteIds() []int64 {
	return append([]int64{u.Id}, u.InviteIds...)
}

// SetPassword 设置密码
func (u *User) SetPassword(password string) {
	u.Password = &Argon2Data{}
	u.Password.Generate(password)
}

// CheckPassword 检查密码
func (u *User) CheckPassword(password string) bool {
	if u == nil || u.Password == nil {
		return false
	}
	return u.Password.Check(password)
}

// SetExtra 设置扩展信息
func (u *User) SetExtra(key string, value any) *User {
	u.Extras, _ = sjson.SetBytes(u.Extras, key, value)
	return u
}

// GetExtra 获取扩展信息
func (u *User) GetExtra(key string) gjson.Result {
	return gjson.Get(string(u.Extras), key)
}
