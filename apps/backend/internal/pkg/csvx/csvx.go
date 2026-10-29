// Package csvx 输出"能直接双击打开"的 CSV。
package csvx

import (
	"encoding/csv"
	"io"
)

// bom 是 UTF-8 的字节序标记。
//
// 必须写：Excel 打开不带 BOM 的 UTF-8 CSV 会按本地代码页解码，
// 中文全变成乱码，而拿到文件的人第一反应是"你们系统导出的东西是坏的"。
var bom = []byte{0xEF, 0xBB, 0xBF}

// Write 把表头与数据行写成带 BOM 的 CSV。
//
// 用 CSV 而不是 xlsx：不引第三方依赖，Excel / Numbers / WPS 都能直接打开，
// 而"把客户名单发给同事一份"这个需求等不起一个新依赖。
func Write(w io.Writer, header []string, rows [][]string) error {
	if _, err := w.Write(bom); err != nil {
		return err
	}

	writer := csv.NewWriter(w)
	if err := writer.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
