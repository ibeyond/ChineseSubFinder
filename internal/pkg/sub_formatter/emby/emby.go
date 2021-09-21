package emby

import (
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/language"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/sub_formatter/common"
	"github.com/allanpk716/ChineseSubFinder/internal/types"
	"path/filepath"
	"regexp"
	"strings"
)

type Formatter struct {
}

func NewFormatter() *Formatter {
	return &Formatter{}
}

// GetFormatterName 当前的 Formatter 是那个
func (f Formatter) GetFormatterName() string {
	return common.FormatterNameString_Emby
}

func (f Formatter) GetFormatterFormatterName() int {
	return int(common.Emby)
}

// IsMatchThisFormat 是否满足当前实现接口的字幕命名格式 - 是否符合规则、fileNameWithOutExt string, subExt string, subLang types.Language, extraSubPreName string
func (f Formatter) IsMatchThisFormat(subName string) (bool, string, string, types.Language, string) {
	/*
		Emby 的命名规则比较特殊，而且本程序就是做中文字幕下载的，所以，下面的正则表达式比较特殊
	*/
	var re = regexp.MustCompile(`(?m).chinese\((\S+)\)(\.\S+)`)
	matched := re.FindAllStringSubmatch(subName, -1)
	/*
		[0][0]	.chinese(简英,subhd).ass
		[0][1]	简英,subhd or 简英
		[0][2]	.ass
	*/
	if matched == nil || len(matched) < 1 || len(matched[0]) < 3 {
		return false, "", "", types.Unknow, ""
	}
	var subLang types.Language
	var subLangStr string
	var extraSubPreName string
	fileNameWithOutExt := strings.ReplaceAll(subName, matched[0][0], "")
	subExt := matched[0][2]
	midString := matched[0][1]
	if strings.Contains(midString, ",") == true {
		tmps := strings.Split(midString, ",")
		if len(tmps) < 2 {
			return false, "", "", types.Unknow, ""
		}
		subLangStr = tmps[0]
		extraSubPreName = tmps[1]
	} else {
		subLangStr = midString
		extraSubPreName = ""
	}
	subLang = language.ChineseString2Lang(subLangStr)

	return true, fileNameWithOutExt, subExt, subLang, extraSubPreName
}

// GenerateMixSubName 通过视频和字幕信息，生成当前实现接口的字幕命名格式。extraSubPreName 一般是填写字幕网站，不填写则留空 - 新名称、新名称带有 default 标记，新名称带有 forced 标记
func (f Formatter) GenerateMixSubName(videoFileName, subExt string, subLang types.Language, extraSubPreName string) (string, string, string) {
	/*
		这里会生成类似的文件名 xxxx.chinese(中英,shooter)
	*/
	videoFileNameWithOutExt := strings.ReplaceAll(filepath.Base(videoFileName),
		filepath.Ext(videoFileName), "")
	return f.GenerateMixSubNameBase(videoFileNameWithOutExt, subExt, subLang, extraSubPreName)
}

func (f Formatter) GenerateMixSubNameBase(fileNameWithOutExt, subExt string, subLang types.Language, extraSubPreName string) (string, string, string) {

	note := ""
	// extraSubPreName 那个字幕网站下载的
	if extraSubPreName != "" {
		note = "," + extraSubPreName
	}

	subNewName := fileNameWithOutExt + types.Emby_chinese + "(" + language.Lang2ChineseString(subLang) + note + ")" + subExt
	subNewNameWithDefault := fileNameWithOutExt + types.Emby_chinese + "(" + language.Lang2ChineseString(subLang) + note + ")" + types.Sub_Ext_Mark_Default + subExt
	subNewNameWithForced := fileNameWithOutExt + types.Emby_chinese + "(" + language.Lang2ChineseString(subLang) + note + ")" + types.Sub_Ext_Mark_Forced + subExt

	return subNewName, subNewNameWithDefault, subNewNameWithForced
}
