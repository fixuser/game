package tag

import "math/bits"

// Enum 约束 protobuf 生成的 Go enum 类型，底层类型为 int32
type Enum interface {
	~int32
}

// Tag 用于位运算的标签类型，每一位代表一个标志
type Tag int64

// Has 检测 tag 是否同时包含所有指定的 flags（AND 语义）
//
//	boot.Has(tags, UserTag_VIP, UserTag_ADMIN) // VIP 和 ADMIN 都存在才返回 true
func Has[E Enum](tag Tag, flags ...E) bool {
	var mask Tag
	for _, f := range flags {
		mask |= Tag(f)
	}
	return tag&mask == mask
}

// HasAny 检测 tag 是否包含任意一个指定的 flags（OR 语义）
//
//	boot.HasAny(tags, UserTag_VIP, UserTag_BANNED) // VIP 或 BANNED 任一存在即返回 true
func HasAny[E Enum](tag Tag, flags ...E) bool {
	var mask Tag
	for _, f := range flags {
		mask |= Tag(f)
	}
	return tag&mask != 0
}

// Set 设置指定的 flags 并返回新的 Tag
//
//	tags = boot.Set(tags, UserTag_VIP, UserTag_ADMIN) // 同时设置 VIP 和 ADMIN
func Set[E Enum](tag Tag, flags ...E) Tag {
	for _, f := range flags {
		tag |= Tag(f)
	}
	return tag
}

// Unset 清除指定的 flags 并返回新的 Tag
//
//	tags = boot.Unset(tags, UserTag_ADMIN) // 移除 ADMIN 标志
func Unset[E Enum](tag Tag, flags ...E) Tag {
	for _, f := range flags {
		tag &^= Tag(f)
	}
	return tag
}

// Toggle 切换指定的 flags（已设置则清除，未设置则添加）并返回新的 Tag
//
//	tags = boot.Toggle(tags, UserTag_VIP) // 切换 VIP 状态
func Toggle[E Enum](tag Tag, flags ...E) Tag {
	for _, f := range flags {
		tag ^= Tag(f)
	}
	return tag
}

// Count 返回当前 Tag 中已设置的位数
func (tag Tag) Count() int {
	return bits.OnesCount64(uint64(tag))
}

// Values 返回 tag 中所有已设置的位对应的 enum 值列表
//
//	vals := boot.Values[UserTag](tags) // 例如返回 [UserTag_VIP, UserTag_ADMIN]
func Values[E Enum](tag Tag) []E {
	result := make([]E, 0, tag.Count())
	for i := 0; i < 64; i++ {
		bit := Tag(1) << i
		if tag&bit != 0 {
			result = append(result, E(int32(bit)))
		}
	}
	return result
}
