package items

import (
	"context"
	"testing"

	"github.com/Linuxea/item-system/item"
)

type stubUsers struct{ last string }

func (s *stubUsers) Rename(_ context.Context, _, n string) error { s.last = n; return nil }

// TestBindDoesNotMutatePrototype 验证 Bind 出的副本带上了参数，而原型保持干净。
// 这是并发安全的前提：多个玩家同时用改名卡，各自拿到自己的副本。
func TestBindDoesNotMutatePrototype(t *testing.T) {
	users := &stubUsers{}
	proto := NewRenameCard(users)

	bound, err := proto.Bind([]byte(`{"new_name":"林夕"}`))
	if err != nil {
		t.Fatalf("Bind 失败: %v", err)
	}
	if proto.NewName != "" {
		t.Fatalf("原型被污染了: %q", proto.NewName)
	}
	card, ok := bound.(*RenameCard)
	if !ok {
		t.Fatalf("Bind 返回的类型不对: %T", bound)
	}
	if card.NewName != "林夕" {
		t.Fatalf("参数没绑上: %q", card.NewName)
	}
	// 依赖必须跟着副本一起带过来，否则 Use 会空指针。
	if card.users == nil {
		t.Fatal("副本丢失了私有依赖")
	}
	if err := card.Use(context.Background(), "u1"); err != nil {
		t.Fatalf("Use 失败: %v", err)
	}
	if users.last != "林夕" {
		t.Fatalf("改名没生效: %q", users.last)
	}
}

// TestBindRejectsBadParams 验证参数校验在 Bind 阶段就拦下，不产生任何副作用。
func TestBindRejectsBadParams(t *testing.T) {
	users := &stubUsers{}
	proto := NewRenameCard(users)

	for _, tc := range []struct {
		name string
		raw  string
		want error
	}{
		{"参数为空", ``, item.ErrMissingParam},
		{"字段缺失", `{}`, item.ErrMissingParam},
		{"空白昵称", `{"new_name":"   "}`, item.ErrMissingParam},
		{"昵称过短", `{"new_name":"林"}`, item.ErrInvalidParam},
		{"昵称过长", `{"new_name":"林夕林夕林夕林夕林夕林夕林夕林夕林夕"}`, item.ErrInvalidParam},
		{"JSON 非法", `{oops`, item.ErrInvalidParam},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := proto.Bind([]byte(tc.raw)); err == nil {
				t.Fatal("本该失败却成功了")
			}
			if users.last != "" {
				t.Fatal("校验阶段不该产生副作用")
			}
		})
	}
}
