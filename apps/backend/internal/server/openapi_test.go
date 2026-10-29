package server

import (
	"bytes"
	"encoding/json"
	"flag"
	"log/slog"
	"os"
	"testing"

	"github.com/edwinhuish/gowork/internal/config"
	"github.com/edwinhuish/gowork/internal/infra/cache"
)

var updateOpenAPI = flag.Bool("update", false, "按当前路由表重新生成 docs/openapi.json")

// 测试放在 internal/server，往上四层才是仓库根的 docs/。
const openAPIFile = "../../../../docs/openapi.json"

func buildOpenAPIFromRoutes(t *testing.T) []byte {
	t.Helper()

	c, err := cache.New(config.Cache{Driver: config.CacheMemory}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("建缓存: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// 空 Handlers 就够：注册路由只取方法值，不解引用，所以不需要真的 service。
	engine := New(&config.Config{}, slog.New(slog.DiscardHandler), nil, nil, c, Handlers{})
	doc := BuildOpenAPI(engine.Routes())

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("序列化文档: %v", err)
	}
	return append(data, '\n')
}

// 这条用例是"路由与文档一致"的唯一保证：加/删/改接口却不重新生成文档，CI 会红。
//
// json.MarshalIndent 对 map 的键排序输出，所以同一份路由表每次生成的字节相同，
// 直接按字节比对即可，不需要额外的规范化。
func TestOpenAPIMatchesRoutes(t *testing.T) {
	data := buildOpenAPIFromRoutes(t)

	if *updateOpenAPI {
		if err := os.WriteFile(openAPIFile, data, 0o644); err != nil {
			t.Fatalf("写入 %s: %v", openAPIFile, err)
		}
		t.Logf("已重新生成 %s（%d 字节）", openAPIFile, len(data))
		return
	}

	existing, err := os.ReadFile(openAPIFile)
	if err != nil {
		t.Fatalf("读取 %s: %v；首次生成请加 -update", openAPIFile, err)
	}
	if !bytes.Equal(existing, data) {
		t.Fatalf("docs/openapi.json 与路由表不一致。" +
			"改动接口后请跑：go test ./internal/server -run TestOpenAPIMatchesRoutes -update")
	}
}
