package boot

import (
	"cmp"
	"path"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/spf13/viper"
)

// Info 包含构建时的版本和 VCS 信息
type Info struct {
	Name      string     // 应用名称（通常是模块路径的最后一部分）
	GoVersion string     // Go 编译器版本，如 "go1.26.1"
	Module    string     // 模块路径
	Version   string     // 模块版本（通常 "(devel)" 表示本地构建）
	VCSType   string     // 版本控制系统类型，如 "git"
	Revision  string     // VCS commit hash
	Time      *time.Time // VCS commit 时间
	Modified  bool       // 工作目录是否有未提交的修改
}

// Read 从 runtime/debug.BuildInfo 中读取构建信息
func Read() Info {
	info := Info{
		GoVersion: runtime.Version(),
	}

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}

	info.Module = bi.Main.Path
	info.Version = bi.Main.Version
	info.Name = cmp.Or(viper.GetString("name"), path.Base(bi.Main.Path))

	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs":
			info.VCSType = s.Value
		case "vcs.revision":
			info.Revision = s.Value
		case "vcs.time":
			if s.Value != "" {
				if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
					info.Time = &t
				}
			}
		case "vcs.modified":
			info.Modified = s.Value == "true"
		}
	}

	return info
}

// Short 返回简短的版本字符串，如 "v1.0.0 (abc1234)"
func (i Info) Short() string {
	s := i.Version
	if i.Revision != "" {
		rev := i.Revision
		if len(rev) > 7 {
			rev = rev[:7]
		}
		s += " (" + rev + ")"
	}
	if i.Modified {
		s += " [dirty]"
	}
	return s
}

// String 返回完整的版本信息
func (i Info) String() string {
	s := i.Module + " " + i.Version
	s += "\n  go:       " + i.GoVersion
	if i.VCSType != "" {
		s += "\n  vcs:      " + i.VCSType
	}
	if i.Revision != "" {
		s += "\n  revision: " + i.Revision
	}
	if i.Time != nil {
		s += "\n  time:     " + i.Time.Format(time.RFC3339)
	}
	if i.Modified {
		s += "\n  modified: true"
	}
	return s
}
