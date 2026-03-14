package models

import (
	"context"
	"slices"
	"time"

	"github.com/game/game/apis/basepb"
	"github.com/game/game/pkg/meta"
	ua "github.com/mileusna/useragent"
	"github.com/uptrace/bun"
)

// LoginLog 登录日志
type LoginLog struct {
	Id        int64                  `bun:",pk,autoincrement"`
	UserId    int64                  `bun:",notnull"`
	Username  string                 `bun:",notnull"`               // 用户名
	Nickname  string                 `bun:",notnull"`               // 昵称
	Ip        string                 `bun:",notnull"`               // 登录IP
	DeviceId  string                 `bun:",nullzero"`              // 登录设备ID
	Model     string                 `bun:",nullzero"`              // 登录设备型号
	Os        string                 `bun:",nullzero"`              // 登录设备操作系统
	Browser   string                 `bun:",nullzero"`              // 登录设备
	Remark    string                 `bun:",nullzero"`              // 备注
	Status    basepb.LoginLog_Status `bun:",notnull,type:smallint"` // 状态
	CreatedAt time.Time              `bun:",nullzero,notnull,default:current_timestamp"`
}

// NewLoginLog 创建登录日志
func NewLoginLog(ctx context.Context, username string) (ll *LoginLog) {
	ll = new(LoginLog)
	ll.Username = username
	ll.FromContext(ctx)
	return
}

func (log *LoginLog) Update(ctx context.Context, db bun.IDB) (err error) {
	err = log.Save(ctx, db)
	if err != nil {
		return
	}

	userLoginDevice := NewUserLoginDevice(log.UserId, log.DeviceId)
	userLoginIp := NewUserLoginIp(log.UserId, log.Ip)
	deviceLoginUser := NewDeviceLoginUser(log.DeviceId, log.UserId)
	ipLoginUser := NewIpLoginUser(log.Ip, log.UserId)
	_, err = db.NewInsert().Model(userLoginDevice).Exec(ctx)
	if err != nil {
		return
	}
	_, err = db.NewInsert().Model(userLoginIp).Exec(ctx)
	if err != nil {
		return
	}
	_, err = db.NewInsert().Model(deviceLoginUser).Exec(ctx)
	if err != nil {
		return
	}
	_, err = db.NewInsert().Model(ipLoginUser).Exec(ctx)
	return
}

// Save 保存登录日志
func (log *LoginLog) Save(ctx context.Context, db bun.IDB) error {
	if log.Status == basepb.LoginLog_STATUS_UNSPECIFIED {
		log.Status = basepb.LoginLog_STATUS_SUCCESS
	}
	_, err := db.NewInsert().Model(log).Exec(ctx)
	return err
}

// FromUser 从用户信息中获取登录日志信息
func (log *LoginLog) FromUser(user *User) *LoginLog {
	if user == nil {
		log.UpdateRemark("account or password failed")
		return log
	}
	log.UserId = user.Id
	log.Nickname = user.Nickname
	log.UpdateRemark("")
	return log
}

// FromContext 从上下文中获取登录日志信息
func (log *LoginLog) FromContext(ctx context.Context) *LoginLog {
	md := meta.FromContext(ctx)
	ua := ua.Parse(md.GetString(meta.MetaUserAgent))
	log.Ip = md.GetString(meta.MetaUserIp)
	log.DeviceId = md.GetString(meta.MetaDeviceId)
	log.Browser = ua.Name
	log.Os = ua.OS
	log.Model = ua.Device
	if log.Model == "" {
		log.Model = log.Os
	}
	return log
}

// UpdateRemark 更新备注
func (log *LoginLog) UpdateRemark(remark string) {
	if log.Remark != "" {
		log.Remark = remark
		log.Status = basepb.LoginLog_STATUS_FAIL
		return
	}
	log.Status = basepb.LoginLog_STATUS_SUCCESS
}

// UserLoginDevice 用户登录设备
type UserLoginDevice struct {
	UserId    int64     `bun:",pk"`
	DeviceIds []string  `bun:",array,notnull"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

// NewUserLoginDevice 创建用户登录设备
func NewUserLoginDevice(userId int64, deviceId string) (uld *UserLoginDevice) {
	uld = new(UserLoginDevice)
	uld.UserId = userId
	uld.DeviceIds = []string{deviceId}
	uld.UpdatedAt = time.Now()
	return
}

// Upsert 更新或插入
func (uld *UserLoginDevice) Upsert(ctx context.Context, db bun.IDB) (err error) {
	_, err = db.NewInsert().Model(uld).On("CONFLICT (user_id) DO UPDATE").
		Set("device_ids = (SELECT array_agg(DISTINCT x) FROM unnest(user_login_device.device_ids || EXCLUDED.device_ids) AS t(x))").
		Set("updated_at = EXCLUDED.updated_at").Exec(ctx)
	return
}

// DeviceLoginUser 设备登录用户
type DeviceLoginUser struct {
	DeviceId  string    `bun:",pk"`
	UserIds   []int64   `bun:",array,notnull"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

// Contains 是否包含用户
func (dlu *DeviceLoginUser) Contains(userId int64) bool {
	return slices.Contains(dlu.UserIds, userId)
}

// NewDeviceLoginUser 创建设备登录用户
func NewDeviceLoginUser(deviceId string, userId int64) (dlu *DeviceLoginUser) {
	dlu = new(DeviceLoginUser)
	dlu.DeviceId = deviceId
	dlu.UserIds = []int64{userId}
	dlu.UpdatedAt = time.Now()
	return
}

// Upsert 更新或插入
func (dlu *DeviceLoginUser) Upsert(ctx context.Context, db bun.IDB) (err error) {
	_, err = db.NewInsert().Model(dlu).On("CONFLICT (device_id) DO UPDATE").
		Set("user_ids = (SELECT array_agg(DISTINCT x) FROM unnest(device_login_user.user_ids || EXCLUDED.user_ids) AS t(x))").
		Set("updated_at = EXCLUDED.updated_at").Exec(ctx)
	return
}

// UserLoginIp 用户登录IP
type UserLoginIp struct {
	UserId    int64     `bun:",pk"`
	Ips       []string  `bun:",array,notnull"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

// NewUserLoginIp 创建用户登录IP
func NewUserLoginIp(userId int64, ip string) (uli *UserLoginIp) {
	uli = new(UserLoginIp)
	uli.UserId = userId
	uli.Ips = []string{ip}
	uli.UpdatedAt = time.Now()
	return
}

// Upsert 更新或插入
func (uli *UserLoginIp) Upsert(ctx context.Context, db bun.IDB) (err error) {
	_, err = db.NewInsert().Model(uli).On("CONFLICT (user_id) DO UPDATE").
		Set("ips = (SELECT array_agg(DISTINCT x) FROM unnest(user_login_ip.ips || EXCLUDED.ips) AS t(x))").
		Set("updated_at = EXCLUDED.updated_at").Exec(ctx)
	return
}

// IpLoginUser IP登录用户
type IpLoginUser struct {
	Ip        string    `bun:",pk"`
	UserIds   []int64   `bun:",array,notnull"`
	UpdatedAt time.Time `bun:",nullzero,notnull,default:current_timestamp"`
}

func (iu *IpLoginUser) Contains(userId int64) bool {
	return slices.Contains(iu.UserIds, userId)
}

// NewIpLoginUser 创建IP登录用户
func NewIpLoginUser(ip string, userId int64) (ilu *IpLoginUser) {
	ilu = new(IpLoginUser)
	ilu.Ip = ip
	ilu.UserIds = []int64{userId}
	ilu.UpdatedAt = time.Now()
	return
}

// Upsert 更新或插入
func (ilu *IpLoginUser) Upsert(ctx context.Context, db bun.IDB) (err error) {
	_, err = db.NewInsert().Model(ilu).On("CONFLICT (ip) DO UPDATE").
		Set("user_ids = (SELECT array_agg(DISTINCT x) FROM unnest(ip_login_user.user_ids || EXCLUDED.user_ids) AS t(x))").
		Set("updated_at = EXCLUDED.updated_at").Exec(ctx)
	return
}
