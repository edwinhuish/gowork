package server

import (
	"strings"

	"github.com/gin-gonic/gin"
)

// 这份文档由路由表生成，不手写。
//
// 用途只有一个：**卡住路由漂移**。谁加/删/改了接口却没重新生成，CI 会失败。
// 因此它只描述"有哪些路径、什么方法、要不要鉴权、响应长什么样"，
// 不展开请求体字段——请求体的权威是各 handler 的 dto，抄一遍必然漂移。
//
// 重新生成：go test ./internal/server -run TestOpenAPIMatchesRoutes -update

// OpenAPIDoc 是生成出来的文档结构。
type OpenAPIDoc struct {
	OpenAPI    string                          `json:"openapi"`
	Info       OpenAPIInfo                     `json:"info"`
	Paths      map[string]map[string]OpenAPIOp `json:"paths"`
	Components OpenAPIComponents               `json:"components"`
}

type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
	// Description 写清这份文档的边界，免得有人拿它当"字段级契约"用。
	Description string `json:"description"`
}

type OpenAPIOp struct {
	OperationID string                     `json:"operationId"`
	Tags        []string                   `json:"tags"`
	Security    []map[string][]string      `json:"security,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
}

// OpenAPIResponse 只放引用：每个接口的响应形状完全相同，
// 展开写会让文档涨到几百 KB，而且改一处要改 158 遍。
type OpenAPIResponse struct {
	Ref         string                      `json:"$ref,omitempty"`
	Description string                      `json:"description,omitempty"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

type OpenAPIMediaType struct {
	Schema map[string]string `json:"schema"`
}

type OpenAPIComponents struct {
	Schemas         map[string]any             `json:"schemas"`
	Responses       map[string]OpenAPIResponse `json:"responses"`
	SecuritySchemes map[string]OpenAPIScheme   `json:"securitySchemes"`
}

type OpenAPIScheme struct {
	Type   string `json:"type"`
	Scheme string `json:"scheme"`
}

// publicPaths 是不需要登录的接口。写死而不是按前缀猜：
// 猜错的后果是文档把公开接口标成需要鉴权（或反过来），两种都是错的。
var publicPaths = map[string]bool{
	"/healthz":           true,
	"/api/v1/auth/login": true,
}

// BuildOpenAPI 把 gin 的路由表转成文档。排序是写死的：
// map 的遍历顺序不确定，不排序的话同一份路由每次生成的字节都不同，
// 校验就失去意义了。
func BuildOpenAPI(routes []gin.RouteInfo) OpenAPIDoc {
	paths := make(map[string]map[string]OpenAPIOp, len(routes))
	for _, r := range routes {
		path := toOpenAPIPath(r.Path)
		if paths[path] == nil {
			paths[path] = map[string]OpenAPIOp{}
		}
		method := strings.ToLower(r.Method)
		op := OpenAPIOp{
			// handler 名里带包路径，只取最后的 `(*XxxHandler).Method-fm`，
			// 拼上方法保证全局唯一（同一个 handler 可能挂两条路由）。
			OperationID: r.Method + "_" + handlerName(r.Handler),
			Tags:        []string{tagOf(path)},
			Responses:   standardResponses(),
		}
		if !publicPaths[r.Path] {
			op.Security = []map[string][]string{{"bearerAuth": {}}}
		}
		paths[path][method] = op
	}

	return OpenAPIDoc{
		OpenAPI: "3.0.3",
		Info: OpenAPIInfo{
			Title:   "GoWork API",
			Version: "1.0.0",
			Description: "由 internal/server 的路由表生成，用途是检出路由漂移。" +
				"请求体字段以各 handler 的 dto 为准，这里不重复描述。",
		},
		Paths: paths,
		Components: OpenAPIComponents{
			Schemas: map[string]any{
				"ApiResponse": map[string]any{
					"type":     "object",
					"required": []string{"code", "message"},
					"properties": map[string]any{
						"code":       map[string]any{"type": "integer", "description": "0 表示成功；非 0 时前三位对齐 HTTP 状态码"},
						"message":    map[string]any{"type": "string"},
						"data":       map[string]any{"description": "业务数据，结构随接口而定；code != 0 时为空"},
						"request_id": map[string]any{"type": "string"},
					},
				},
			},
			Responses: sharedResponses(),
			SecuritySchemes: map[string]OpenAPIScheme{
				"bearerAuth": {Type: "http", Scheme: "bearer"},
			},
		},
	}
}

// standardResponses 返回"只含 $ref"的状态码表。内容定义在 components.responses 里。
func standardResponses() map[string]OpenAPIResponse {
	ref := func(name string) OpenAPIResponse {
		return OpenAPIResponse{Ref: "#/components/responses/" + name}
	}
	return map[string]OpenAPIResponse{
		"200": ref("Ok"),
		"400": ref("BadRequest"),
		"401": ref("Unauthorized"),
		"403": ref("Forbidden"),
		"404": ref("NotFound"),
		"409": ref("Conflict"),
		"500": ref("Internal"),
	}
}

func sharedResponses() map[string]OpenAPIResponse {
	resp := func(desc string) OpenAPIResponse {
		return OpenAPIResponse{
			Description: desc,
			Content: map[string]OpenAPIMediaType{
				"application/json": {
					Schema: map[string]string{"$ref": "#/components/schemas/ApiResponse"},
				},
			},
		}
	}
	return map[string]OpenAPIResponse{
		"Ok":           resp("成功（新建也可能返回 200，统一响应体里 code=0）"),
		"BadRequest":   resp("参数不合法，code=40001"),
		"Unauthorized": resp("未登录或令牌过期，code=40101/40102"),
		"Forbidden":    resp("没有权限，code=40301"),
		"NotFound":     resp("资源不存在，code=40401"),
		"Conflict":     resp("状态冲突或数据重复，code=40901"),
		"Internal":     resp("服务器内部错误，code=50001"),
	}
}

// toOpenAPIPath 把 gin 的 `:id` 参数换成 OpenAPI 的 `{id}`。
func toOpenAPIPath(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, ":") {
			segs[i] = "{" + strings.TrimPrefix(s, ":") + "}"
		}
	}
	return strings.Join(segs, "/")
}

// handlerName 从 gin 的 handler 标识里取出可读的方法名。
//
// gin 给的是完整符号，形如：
//
//	github.com/edwinhuish/gowork/internal/handler.(*ProductHandler).List-fm
func handlerName(symbol string) string {
	if idx := strings.LastIndex(symbol, "/"); idx >= 0 {
		symbol = symbol[idx+1:]
	}
	symbol = strings.TrimPrefix(symbol, "(")
	symbol = strings.ReplaceAll(symbol, "(*", "")
	symbol = strings.ReplaceAll(symbol, ")-fm", "")
	symbol = strings.ReplaceAll(symbol, ")", "")
	if symbol == "" {
		return "unknown"
	}
	// operationId 里不放点号：规范只要求唯一，但不少代码生成器把它当标识符用。
	return strings.ReplaceAll(symbol, ".", "_")
}

// tagOf 取路径里的资源名作为分组：/api/v1/shipments/{id}/invoice → shipments。
func tagOf(path string) string {
	segs := strings.Split(strings.Trim(path, "/"), "/")
	for i, s := range segs {
		if s == "v1" && i+1 < len(segs) {
			return segs[i+1]
		}
	}
	if len(segs) > 0 {
		return segs[0]
	}
	return "default"
}
