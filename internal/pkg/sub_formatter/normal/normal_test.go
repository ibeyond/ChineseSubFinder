package normal

import (
	"github.com/allanpk716/ChineseSubFinder/internal/common"
	subCommon "github.com/allanpk716/ChineseSubFinder/internal/pkg/sub_formatter/common"
	"github.com/allanpk716/ChineseSubFinder/internal/types"
	"testing"
)

func TestFormatter_GetFormatterName(t *testing.T) {
	f := NewFormatter()
	if f.GetFormatterName() != subCommon.FormatterNameString_Normal {
		t.Errorf("GetFormatterName error")
	}
}

func TestFormatter_IsMatchThisFormat(t *testing.T) {

	const fileWithOutExt = "The Boss Baby Family Business (2021) WEBDL-1080p"

	type args struct {
		subName string
	}
	tests := []struct {
		name  string
		args  args
		want  bool
		want1 string
		want2 string
		want3 types.Language
		want4 string
	}{
		{name: "00", args: args{subName: "The Boss Baby Family Business (2021) WEBDL-1080p.zh.ass"},
			want:  true,
			want1: fileWithOutExt,
			want2: ".ass",
			want3: types.ChineseSimple,
			want4: ""},
		{name: "01", args: args{subName: "The Boss Baby Family Business (2021) WEBDL-1080p.zh.default.ass"},
			want:  true,
			want1: fileWithOutExt,
			want2: ".default.ass",
			want3: types.ChineseSimple,
			want4: ""},
		{name: "02", args: args{subName: "The Boss Baby Family Business (2021) WEBDL-1080p.zh.forced.ass"},
			want:  true,
			want1: fileWithOutExt,
			want2: ".forced.ass",
			want3: types.ChineseSimple,
			want4: ""},
		{name: "03", args: args{subName: "The Boss Baby Family Business (2021) WEBDL-1080p.cn.ass"},
			want:  false,
			want1: "",
			want2: "",
			want3: types.Unknow,
			want4: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Formatter{}
			got, got1, got2, got3, got4 := f.IsMatchThisFormat(tt.args.subName)
			if got != tt.want {
				t.Errorf("IsMatchThisFormat() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("IsMatchThisFormat() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("IsMatchThisFormat() got2 = %v, want %v", got2, tt.want2)
			}
			if got3 != tt.want3 {
				t.Errorf("IsMatchThisFormat() got3 = %v, want %v", got3, tt.want3)
			}
			if got4 != tt.want4 {
				t.Errorf("IsMatchThisFormat() got4 = %v, want %v", got4, tt.want4)
			}
		})
	}
}

func TestFormatter_GenerateMixSubName(t *testing.T) {

	const videoFileName = "Django Unchained (2012) Bluray-1080p.mp4"
	const videoFileNamePre = "Django Unchained (2012) Bluray-1080p"

	type args struct {
		videoFileName   string
		subExt          string
		subLang         types.Language
		extraSubPreName string
	}
	tests := []struct {
		name  string
		args  args
		want  string
		want1 string
		want2 string
	}{
		{name: "zh", args: args{videoFileName: videoFileName, subExt: common.SubExtASS, subLang: types.ChineseSimple, extraSubPreName: ""},
			want:  videoFileNamePre + ".zh.ass",
			want1: videoFileNamePre + ".zh.default.ass",
			want2: videoFileNamePre + ".zh.forced.ass"},
		{name: "zh_shooter", args: args{videoFileName: videoFileName, subExt: common.SubExtASS, subLang: types.ChineseSimple, extraSubPreName: "shooter"},
			want:  videoFileNamePre + ".zh.ass",
			want1: videoFileNamePre + ".zh.default.ass",
			want2: videoFileNamePre + ".zh.forced.ass"},
		{name: "zh_shooter2", args: args{videoFileName: videoFileName, subExt: common.SubExtASS, subLang: types.ChineseSimpleEnglish, extraSubPreName: "shooter"},
			want:  videoFileNamePre + ".zh.ass",
			want1: videoFileNamePre + ".zh.default.ass",
			want2: videoFileNamePre + ".zh.forced.ass"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Formatter{}
			got, got1, got2 := f.GenerateMixSubName(tt.args.videoFileName, tt.args.subExt, tt.args.subLang, tt.args.extraSubPreName)
			if got != tt.want {
				t.Errorf("GenerateMixSubName() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("GenerateMixSubName() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("GenerateMixSubName() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}
