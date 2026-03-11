package tag

import (
	"testing"
)

// 模拟 protobuf 生成的 enum 类型
type mockTag int32

const (
	mockTag_VIP       mockTag = 1  // 第1位
	mockTag_VERIFIED  mockTag = 2  // 第2位
	mockTag_ADMIN     mockTag = 4  // 第3位
	mockTag_BANNED    mockTag = 8  // 第4位
	mockTag_DEVELOPER mockTag = 16 // 第5位
)

func TestSet(t *testing.T) {
	var tags Tag

	tags = Set(tags, mockTag_VIP)
	if tags != 1 {
		t.Fatalf("expected 1, got %d", tags)
	}

	tags = Set(tags, mockTag_ADMIN)
	if tags != 5 {
		t.Fatalf("expected 5, got %d", tags)
	}
}

func TestSetMultiple(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN, mockTag_DEVELOPER)
	if tags != 21 {
		t.Fatalf("expected 21, got %d", tags)
	}
}

func TestSetIdempotent(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP)
	tags = Set(tags, mockTag_VIP) // 重复设置
	if tags != 1 {
		t.Fatalf("expected 1 (idempotent), got %d", tags)
	}
}

func TestHas(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN)

	if !Has(tags, mockTag_VIP) {
		t.Fatal("expected Has(VIP) = true")
	}
	if !Has(tags, mockTag_ADMIN) {
		t.Fatal("expected Has(ADMIN) = true")
	}
	if !Has(tags, mockTag_VIP, mockTag_ADMIN) {
		t.Fatal("expected Has(VIP, ADMIN) = true")
	}
	if Has(tags, mockTag_VIP, mockTag_BANNED) {
		t.Fatal("expected Has(VIP, BANNED) = false")
	}
	if Has(tags, mockTag_BANNED) {
		t.Fatal("expected Has(BANNED) = false")
	}
}

func TestHasAny(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN)

	if !HasAny(tags, mockTag_VIP, mockTag_BANNED) {
		t.Fatal("expected HasAny(VIP, BANNED) = true")
	}
	if !HasAny(tags, mockTag_ADMIN) {
		t.Fatal("expected HasAny(ADMIN) = true")
	}
	if HasAny(tags, mockTag_BANNED, mockTag_DEVELOPER) {
		t.Fatal("expected HasAny(BANNED, DEVELOPER) = false")
	}
}

func TestUnset(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN, mockTag_BANNED)

	tags = Unset(tags, mockTag_ADMIN)
	if Has(tags, mockTag_ADMIN) {
		t.Fatal("expected ADMIN to be unset")
	}
	if !Has(tags, mockTag_VIP, mockTag_BANNED) {
		t.Fatal("expected VIP and BANNED to remain")
	}
}

func TestUnsetMultiple(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN, mockTag_BANNED)
	tags = Unset(tags, mockTag_VIP, mockTag_BANNED)

	if tags != 4 {
		t.Fatalf("expected 4, got %d", tags)
	}
}

func TestUnsetNotPresent(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP)
	tags = Unset(tags, mockTag_BANNED) // 清除不存在的位

	if tags != 1 {
		t.Fatalf("expected 1 (unchanged), got %d", tags)
	}
}

func TestToggle(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP)

	// 切换：VIP 已存在 → 清除，ADMIN 不存在 → 设置
	tags = Toggle(tags, mockTag_VIP, mockTag_ADMIN)
	if Has(tags, mockTag_VIP) {
		t.Fatal("expected VIP to be toggled off")
	}
	if !Has(tags, mockTag_ADMIN) {
		t.Fatal("expected ADMIN to be toggled on")
	}
}

func TestToggleTwice(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP)
	tags = Toggle(tags, mockTag_VIP)
	tags = Toggle(tags, mockTag_VIP)

	if !Has(tags, mockTag_VIP) {
		t.Fatal("expected VIP to be restored after double toggle")
	}
}

func TestCount(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN, mockTag_DEVELOPER)
	if tags.Count() != 3 {
		t.Fatalf("expected count 3, got %d", tags.Count())
	}
}

func TestCountZero(t *testing.T) {
	var tags Tag
	if tags.Count() != 0 {
		t.Fatalf("expected count 0, got %d", tags.Count())
	}
}

func TestValues(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP, mockTag_ADMIN, mockTag_DEVELOPER)
	vals := Values[mockTag](tags)

	if len(vals) != 3 {
		t.Fatalf("expected 3 values, got %d", len(vals))
	}

	expected := map[mockTag]bool{
		mockTag_VIP:       false,
		mockTag_ADMIN:     false,
		mockTag_DEVELOPER: false,
	}
	for _, v := range vals {
		if _, ok := expected[v]; !ok {
			t.Fatalf("unexpected value: %d", v)
		}
		expected[v] = true
	}
	for k, found := range expected {
		if !found {
			t.Fatalf("missing expected value: %d", k)
		}
	}
}

func TestValuesEmpty(t *testing.T) {
	vals := Values[mockTag](Tag(0))
	if len(vals) != 0 {
		t.Fatalf("expected 0 values, got %d", len(vals))
	}
}

func TestHasEmptyFlags(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP)

	// 空 flags 的 mask = 0，tag & 0 == 0 恒成立
	if !Has[mockTag](tags) {
		t.Fatal("expected Has with no flags = true")
	}
}

func TestHasAnyEmptyFlags(t *testing.T) {
	tags := Set(Tag(0), mockTag_VIP)

	// 空 flags 的 mask = 0，tag & 0 != 0 恒不成立
	if HasAny[mockTag](tags) {
		t.Fatal("expected HasAny with no flags = false")
	}
}

func TestCombinedOperations(t *testing.T) {
	var tags Tag

	// 设置 VIP + VERIFIED + ADMIN
	tags = Set(tags, mockTag_VIP, mockTag_VERIFIED, mockTag_ADMIN)
	if tags != 7 {
		t.Fatalf("expected 7, got %d", tags)
	}

	// 移除 VERIFIED
	tags = Unset(tags, mockTag_VERIFIED)
	if tags != 5 {
		t.Fatalf("expected 5, got %d", tags)
	}

	// 切换 ADMIN（清除）和 BANNED（设置）
	tags = Toggle(tags, mockTag_ADMIN, mockTag_BANNED)
	if tags != 9 {
		t.Fatalf("expected 9, got %d", tags)
	}

	// 最终状态：VIP + BANNED
	if !Has(tags, mockTag_VIP, mockTag_BANNED) {
		t.Fatal("expected VIP and BANNED")
	}
	if Has(tags, mockTag_ADMIN) {
		t.Fatal("expected ADMIN to be removed")
	}
	if tags.Count() != 2 {
		t.Fatalf("expected count 2, got %d", tags.Count())
	}
}
