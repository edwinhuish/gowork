package pdf

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/image/font/sfnt"
)

// fontCandidates 是常见 Linux 发行版上带中文的 **TrueType** 字体。
//
// 只列 .ttf：.ttc 是字体集合（一个文件里装多套字），gopdf 不认；
// 而 macOS 与 Windows 的中文字体多为 .ttc 或 .otf，那两个平台要靠配置显式给一个。
//
// DroidSansFallback 排在最后：它是 Android 的"回退字体"，**只含汉字没有拉丁字母**，
// 用它打印出来的单据上，单号、金额与英文条款会整段消失（见 CheckFont）。
var fontCandidates = []string{
	"/usr/share/fonts/truetype/wqy/wqy-microhei.ttf",
	"/usr/share/fonts/windows11-fonts/simhei.ttf",
	"/usr/share/fonts/truetype/arphic/uming.ttf",
	"/usr/share/fonts/truetype/droid/DroidSansFallbackFull.ttf",
	"/usr/share/fonts/truetype/droid/DroidSansFallback.ttf",
}

// probeRunes 是判断字体覆盖面的样本。
//
// 只挑两类：拉丁字母数字（单号、金额、英文条款全靠它）与几个最常出现的汉字。
// 不做全字表校验——汉字有几万个，任何字体都会缺一部分，
// 而"缺一个生僻字"与"整个英文消失"是两回事，后者才是这套默认字体要挡住的。
const probeRunes = "Aa0.-()" + "中文报价单合同发票合计数量单价金额备注客户"

// DetectFont 返回第一个同时能画拉丁与汉字的字体路径；都不合格时返回空串。
func DetectFont() string {
	for _, p := range fontCandidates {
		if CheckFont(p) == nil {
			return p
		}
	}
	return ""
}

// CheckFont 校验字体能同时画出拉丁与汉字。
//
// 返回错误而不是布尔：调用方要把"为什么不用这个字体"写进日志——
// 否则换一台机器打印出来的单子英文全没了，只能靠翻代码猜到 Droid 那个坑。
func CheckFont(path string) error {
	_, err := loadFont(path)
	return err
}

// loadFont 读字体并校验，返回可交给 gopdf 的字节。
func loadFont(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取字体 %s: %w", path, err)
	}
	f, err := sfnt.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("解析字体 %s（需 TrueType .ttf，不支持 .ttc 集合）: %w", path, err)
	}

	var buf sfnt.Buffer
	missing := make([]string, 0, 8)
	for _, r := range probeRunes {
		idx, err := f.GlyphIndex(&buf, r)
		if err != nil || idx == 0 {
			if len(missing) < 8 {
				missing = append(missing, string(r))
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("字体 %s 缺少字形 %s（只含汉字的回退字体不能用于打印，英文与数字会整段消失）",
			path, strings.Join(missing, " "))
	}
	return b, nil
}
