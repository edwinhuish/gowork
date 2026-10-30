package bootstrap_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"

	"github.com/edwinhuish/gowork/internal/model"
)

// 产品图片整条链路：路由挂载、multipart 字段名、主图推导、排序 409、取图鉴权与 ETag。
//
// 单测按 service 传结构体，碰不到「multipart 字段叫 files 还是 file」「路由注册顺序」
// 这类只在 HTTP 侧才会错的事；而它们错了的表现是前端一个请求都发不出去。
func TestE2EProductImages(t *testing.T) {
	_, srv := newE2EApp(t)
	token := login(t, srv)

	// 1. 建一个产品。
	res := call(t, srv, http.MethodPost, "/api/v1/products", token,
		[]byte(`{"sku_code":"IMG-0001","name_cn":"图片测试件","unit":"PCS","status":1}`))
	if res.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("建产品失败: %d %s", res.StatusCode, msg)
	}
	_, _, data := decodeEnvelope(t, res)
	var product model.Product
	if err := json.Unmarshal(data, &product); err != nil {
		t.Fatalf("产品响应格式不对: %s", data)
	}

	// 2. 一次传两张：第一张必须自动成为主图。
	pngBytes := func(c color.Color) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 60, 40))
		draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			t.Fatalf("生成 PNG: %v", err)
		}
		return buf.Bytes()
	}

	upload := func(names []string, contents [][]byte) *http.Response {
		t.Helper()
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		for i, name := range names {
			part, err := w.CreateFormFile("files", name)
			if err != nil {
				t.Fatalf("构造 multipart 失败: %v", err)
			}
			if _, err := part.Write(contents[i]); err != nil {
				t.Fatalf("写图片内容失败: %v", err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatalf("关闭 multipart: %v", err)
		}

		url := srv.URL + "/api/v1/products/" + strconv.FormatUint(uint64(product.ID), 10) + "/images"
		req, err := http.NewRequest(http.MethodPost, url, &buf)
		if err != nil {
			t.Fatalf("构造请求失败: %v", err)
		}
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+token)
		out, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("上传失败: %v", err)
		}
		return out
	}

	type imageItem struct {
		ID        uint   `json:"id"`
		SortOrder int    `json:"sort_order"`
		IsPrimary bool   `json:"is_primary"`
		URL       string `json:"url"`
		ThumbURL  string `json:"thumb_url"`
		MimeType  string `json:"mime_type"`
	}

	uploadRes := upload(
		[]string{"正面.png", "背面.png"},
		[][]byte{pngBytes(color.RGBA{R: 200, A: 255}), pngBytes(color.RGBA{B: 200, A: 255})},
	)
	if uploadRes.StatusCode != http.StatusCreated {
		_, msg, _ := decodeEnvelope(t, uploadRes)
		t.Fatalf("上传图片失败: %d %s", uploadRes.StatusCode, msg)
	}
	_, _, data = decodeEnvelope(t, uploadRes)
	var images []imageItem
	if err := json.Unmarshal(data, &images); err != nil || len(images) != 2 {
		t.Fatalf("上传应返回 2 张，实际 %s", data)
	}
	if !images[0].IsPrimary || images[1].IsPrimary {
		t.Fatalf("第一张应为主图，实际 %+v", images)
	}
	if images[0].ThumbURL == "" {
		t.Fatalf("应回填缩略图路径: %+v", images[0])
	}

	// 3. 产品列表带出主图：列表页的缩略图靠这个字段，不能逐条再请求。
	res = call(t, srv, http.MethodGet, "/api/v1/products?keyword=IMG-0001", token, nil)
	_, _, data = decodeEnvelope(t, res)
	if !bytes.Contains(data, []byte(`"primary_image"`)) {
		t.Fatalf("产品列表应带主图，实际 %s", data)
	}

	// 4. 倒序排列：第三张变主图（这里只有两张，第二张接任）。
	orderBody, _ := json.Marshal(map[string]any{
		"ids": []uint{images[1].ID, images[0].ID},
	})
	res = call(t, srv, http.MethodPut,
		"/api/v1/products/"+strconv.FormatUint(uint64(product.ID), 10)+"/images/order", token, orderBody)
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("调整顺序失败: %d %s", res.StatusCode, msg)
	}
	_, _, data = decodeEnvelope(t, res)
	var reordered []imageItem
	if err := json.Unmarshal(data, &reordered); err != nil {
		t.Fatalf("排序响应格式不对: %s", data)
	}
	if reordered[0].ID != images[1].ID || !reordered[0].IsPrimary {
		t.Fatalf("倒序后应由第二张当主图，实际 %+v", reordered)
	}

	// 集合不一致（少一个 id）必须是 409：用户看到的是旧列表。
	badOrder, _ := json.Marshal(map[string]any{"ids": []uint{images[1].ID}})
	res = call(t, srv, http.MethodPut,
		"/api/v1/products/"+strconv.FormatUint(uint64(product.ID), 10)+"/images/order", token, badOrder)
	if res.StatusCode != http.StatusConflict {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("集合不一致应 409，实际 %d %s", res.StatusCode, msg)
	} else {
		_, _, _ = decodeEnvelope(t, res)
	}

	// 5. 取缩略图：必须带令牌；ETag 命中时 304，不重传字节。
	thumbURL := srv.URL + "/api/v1" + reordered[0].ThumbURL
	req, _ := http.NewRequest(http.MethodGet, thumbURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	thumbRes, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("取缩略图失败: %v", err)
	}
	if thumbRes.StatusCode != http.StatusOK {
		t.Fatalf("取缩略图失败: %d", thumbRes.StatusCode)
	}
	if ct := thumbRes.Header.Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("缩略图应为 image/jpeg，实际 %s", ct)
	}
	etag := thumbRes.Header.Get("ETag")
	if etag == "" {
		t.Fatal("应带 ETag")
	}
	_, _ = thumbRes.Body.Read(make([]byte, 8))
	_ = thumbRes.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, thumbURL, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("If-None-Match", etag)
	notModified, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("带 If-None-Match 取图失败: %v", err)
	}
	_ = notModified.Body.Close()
	if notModified.StatusCode != http.StatusNotModified {
		t.Fatalf("ETag 命中应 304，实际 %d", notModified.StatusCode)
	}

	// 6. 删除主图，下一张自动接任。
	res = call(t, srv, http.MethodDelete,
		"/api/v1/products/"+strconv.FormatUint(uint64(product.ID), 10)+"/images/"+strconv.FormatUint(uint64(reordered[0].ID), 10), token, nil)
	if res.StatusCode != http.StatusOK {
		_, msg, _ := decodeEnvelope(t, res)
		t.Fatalf("删除图片失败: %d %s", res.StatusCode, msg)
	} else {
		_, _, _ = decodeEnvelope(t, res)
	}
	res = call(t, srv, http.MethodGet,
		"/api/v1/products/"+strconv.FormatUint(uint64(product.ID), 10)+"/images", token, nil)
	_, _, data = decodeEnvelope(t, res)
	var left []imageItem
	if err := json.Unmarshal(data, &left); err != nil || len(left) != 1 || !left[0].IsPrimary {
		t.Fatalf("删除主图后应由剩下那张接任，实际 %s", data)
	}

	// 7. 改了后缀的非图片：400，并且提示是「内容不是 image/png」。
	badRes := upload([]string{"假的.png"}, [][]byte{[]byte("这不是图片")})
	if badRes.StatusCode != http.StatusBadRequest {
		_, msg, _ := decodeEnvelope(t, badRes)
		t.Fatalf("伪装文件应 400，实际 %d %s", badRes.StatusCode, msg)
	}
	_, msg, _ := decodeEnvelope(t, badRes)
	if !bytes.Contains([]byte(msg), []byte("不是")) {
		t.Errorf("提示应说明内容与扩展名不符，实际 %q", msg)
	}
}
