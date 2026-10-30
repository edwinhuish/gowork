package server

import (
	"regexp"
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
			// operationId 由「方法 + 路径」生成，不取 handler 的函数名：
			// gin 给的名字是编译器生成的，闭包那部分跨 Go 版本会变
			// （1.27 是 handler.PartnerHandler.List.func1，1.26 是
			// server.New.PartnerHandler.List.func19 —— 归属和序号都不同）。
			// 于是同一份路由在不同工具链下生成的文档不一样：go.mod 写的是
			// 1.26，CI 装的也是 1.26，本地却是 1.27 —— 表现为"本地过、CI 不过"。
			// 路径是写死在注册代码里的，跟编译器无关。
			// 同一个 handler 挂两条路由时路径不同，唯一性照样成立。
			OperationID: operationID(r.Method, path),
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

// repeatedUnderscores 用于把 /api/v1/x/{id} 转换后连着的下划线并成一个。
var repeatedUnderscores = regexp.MustCompile(`_+`)

// operationID 用「方法 + 路径」拼 operationId，形如
// GET_api_v1_partners_id_addresses。
//
// 分隔符与 {参数} 的花括号都换成下划线：规范只要求 operationId 唯一，
// 但不少代码生成器把它当标识符用，点号 / 斜杠 / 花括号都不该出现。
func operationID(method, path string) string {
	replaced := strings.NewReplacer("/", "_", "{", "_", "}", "_", ".", "_", "-", "_").Replace(
		strings.Trim(path, "/"),
	)
	// 末尾的 {id} 会留下一个悬空下划线（…/activities/{id} → activities_id_），
	// 前后都修掉。
	return method + "_" + strings.Trim(repeatedUnderscores.ReplaceAllString(replaced, "_"), "_")
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
