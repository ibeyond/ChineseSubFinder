package subhd

import (
	"bytes"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"github.com/Tnze/go.num/v2/zh"
	"github.com/allanpk716/ChineseSubFinder/internal/common"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/decode"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/log_helper"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/notify_center"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/rod_helper"
	"github.com/allanpk716/ChineseSubFinder/internal/pkg/sub_helper"
	"github.com/allanpk716/ChineseSubFinder/internal/types"
	"github.com/allanpk716/ChineseSubFinder/internal/types/series"
	"github.com/allanpk716/ChineseSubFinder/internal/types/supplier"
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/nfnt/resize"
	"github.com/sirupsen/logrus"
	"image/jpeg"
	"io/ioutil"
	"math"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Supplier struct {
	reqParam    types.ReqParam
	log         *logrus.Logger
	topic       int
	rodLauncher *launcher.Launcher
	tt          time.Duration
}

func NewSupplier(_reqParam ...types.ReqParam) *Supplier {

	sup := Supplier{}
	sup.log = log_helper.GetLogger()
	sup.topic = common.DownloadSubsPerSite
	if len(_reqParam) > 0 {
		sup.reqParam = _reqParam[0]
		if sup.reqParam.Topic > 0 && sup.reqParam.Topic != sup.topic {
			sup.topic = sup.reqParam.Topic
		}
	}

	// 默认超时是 2 * 60s，如果是调试模式则是 5 min
	sup.tt = common.HTMLTimeOut
	if sup.reqParam.DebugMode == true {
		sup.tt = common.OneVideoProcessTimeOut
	}

	return &sup
}

func (s Supplier) GetSupplierName() string {
	return common.SubSiteSubHd
}

func (s Supplier) GetReqParam() types.ReqParam {
	return s.reqParam
}

func (s Supplier) GetSubListFromFile4Movie(filePath string) ([]supplier.SubInfo, error) {
	return s.getSubListFromFile4Movie(filePath)
}

func (s Supplier) GetSubListFromFile4Series(seriesInfo *series.SeriesInfo) ([]supplier.SubInfo, error) {

	var browser *rod.Browser
	// TODO 是用本地的 Browser 还是远程的，推荐是远程的
	browser, err := rod_helper.NewBrowser(s.reqParam.HttpProxy, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = browser.Close()
	}()
	var subInfos = make([]supplier.SubInfo, 0)
	var subList = make([]HdListItem, 0)
	for value := range seriesInfo.NeedDlSeasonDict {
		// 第一级界面，找到影片的详情界面
		keyword := seriesInfo.Name + " 第" + zh.Uint64(value).String() + "季"
		detailPageUrl, err := s.step0(browser, keyword)
		if err != nil {
			s.log.Errorln("subhd step0", keyword)
			return nil, err
		}
		if detailPageUrl == "" {
			// 如果只是搜索不到，则继续换关键词
			s.log.Warning("subhd first search keyword", keyword, "not found")
			keyword = seriesInfo.Name
			s.log.Warning("subhd Retry", keyword)
			detailPageUrl, err = s.step0(browser, keyword)
			if err != nil {
				s.log.Errorln("subhd step0", keyword)
				return nil, err
			}
		}
		if detailPageUrl == "" {
			s.log.Warning("subhd search keyword", keyword, "not found")
			continue
		}
		// 列举字幕
		oneSubList, err := s.step1(browser, detailPageUrl, false)
		if err != nil {
			s.log.Errorln("subhd step1", keyword)
			return nil, err
		}

		subList = append(subList, oneSubList...)
	}
	// 与剧集需要下载的集 List 进行比较，找到需要下载的列表
	// 找到那些 Eps 需要下载字幕的
	subInfoNeedDownload := s.whichEpisodeNeedDownloadSub(seriesInfo, subList)
	// 下载字幕
	for i, item := range subInfoNeedDownload {
		hdContent, err := s.step2Ex(browser, item.Url)
		if err != nil {
			s.log.Errorln("subhd step2Ex", err)
			continue
		}
		oneSubInfo := supplier.NewSubInfo(s.GetSupplierName(), int64(i), hdContent.Filename, types.ChineseSimple, pkg.AddBaseUrl(common.SubSubHDRootUrl, item.Url), 0,
			0, hdContent.Ext, hdContent.Data)
		oneSubInfo.Season = item.Season
		oneSubInfo.Episode = item.Episode
		subInfos = append(subInfos, *oneSubInfo)
	}

	return subInfos, nil
}

func (s Supplier) GetSubListFromFile4Anime(seriesInfo *series.SeriesInfo) ([]supplier.SubInfo, error) {
	panic("not implemented")
}

func (s Supplier) getSubListFromFile4Movie(filePath string) ([]supplier.SubInfo, error) {
	/*
		虽然是传入视频文件路径，但是其实需要读取对应的视频文件目录下的
		movie.xml 以及 *.nfo，找到 IMDB id
		优先通过 IMDB id 去查找字幕
		如果找不到，再靠文件名提取影片名称去查找
	*/
	// 得到这个视频文件名中的信息
	info, _, err := decode.GetVideoInfoFromFileFullPath(filePath)
	if err != nil {
		return nil, err
	}
	// 找到这个视频文件，尝试得到 IMDB ID
	// 目前测试来看，加入 年 这个关键词去搜索，对 2020 年后的影片有利，因为网站有统一的详细页面了，而之前的，没有，会影响识别
	// 所以，year >= 2020 年，则可以多加一个关键词（年）去搜索影片
	imdbInfo, err := decode.GetImdbInfo4Movie(filePath)
	if err != nil {
		// 允许的错误，跳过，继续进行文件名的搜索
		s.log.Errorln("model.GetImdbInfo", err)
	}
	var subInfoList []supplier.SubInfo

	if imdbInfo.ImdbId != "" {
		// 先用 imdb id 找
		subInfoList, err = s.getSubListFromKeyword4Movie(imdbInfo.ImdbId)
		if err != nil {
			// 允许的错误，跳过，继续进行文件名的搜索
			s.log.Errorln(s.GetSupplierName(), "keyword:", imdbInfo.ImdbId)
			s.log.Errorln("getSubListFromKeyword4Movie", "IMDBID can not found sub", filePath, err)
		}
		// 如果有就优先返回
		if len(subInfoList) > 0 {
			return subInfoList, nil
		}
	}
	// 如果没有，那么就用文件名查找
	searchKeyword := pkg.VideoNameSearchKeywordMaker(info.Title, imdbInfo.Year)
	subInfoList, err = s.getSubListFromKeyword4Movie(searchKeyword)
	if err != nil {
		s.log.Errorln(s.GetSupplierName(), "keyword:", searchKeyword)
		return nil, err
	}

	return subInfoList, nil
}

func (s Supplier) getSubListFromKeyword4Movie(keyword string) ([]supplier.SubInfo, error) {

	var browser *rod.Browser
	// TODO 是用本地的 Browser 还是远程的，推荐是远程的
	browser, err := rod_helper.NewBrowser(s.reqParam.HttpProxy, true)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = browser.Close()
	}()
	var subInfos []supplier.SubInfo
	detailPageUrl, err := s.step0(browser, keyword)
	if err != nil {
		return nil, err
	}
	// 没有搜索到字幕
	if detailPageUrl == "" {
		return nil, nil
	}
	subList, err := s.step1(browser, detailPageUrl, true)
	if err != nil {
		return nil, err
	}

	for i, item := range subList {
		hdContent, err := s.step2Ex(browser, item.Url)
		time.Sleep(time.Second)
		if err != nil {
			s.log.Errorln("subhd step2Ex", err)
			return nil, err
		}
		subInfos = append(subInfos, *supplier.NewSubInfo(s.GetSupplierName(), int64(i), hdContent.Filename, types.ChineseSimple, pkg.AddBaseUrl(common.SubSubHDRootUrl, item.Url), 0, 0, hdContent.Ext, hdContent.Data))
	}

	return subInfos, nil
}

func (s Supplier) whichEpisodeNeedDownloadSub(seriesInfo *series.SeriesInfo, allSubList []HdListItem) []HdListItem {
	// 字幕很多，考虑效率，需要做成字典
	// key SxEx - SubInfos
	var allSubDict = make(map[string][]HdListItem)
	// 全季的字幕列表
	var oneSeasonSubDict = make(map[string][]HdListItem)
	for _, subInfo := range allSubList {
		_, season, episode, err := decode.GetSeasonAndEpisodeFromSubFileName(subInfo.Title)
		if err != nil {
			s.log.Errorln("whichEpisodeNeedDownloadSub.GetVideoInfoFromFileFullPath", subInfo.Title, err)
			continue
		}
		subInfo.Season = season
		subInfo.Episode = episode
		epsKey := pkg.GetEpisodeKeyName(season, episode)
		_, ok := allSubDict[epsKey]
		if ok == false {
			// 初始化
			allSubDict[epsKey] = make([]HdListItem, 0)
			if season != 0 && episode == 0 {
				oneSeasonSubDict[epsKey] = make([]HdListItem, 0)
			}
		}
		// 添加
		allSubDict[epsKey] = append(allSubDict[epsKey], subInfo)
		if season != 0 && episode == 0 {
			oneSeasonSubDict[epsKey] = append(oneSeasonSubDict[epsKey], subInfo)
		}
	}
	// 本地的视频列表，找到没有字幕的
	// 需要进行下载字幕的列表
	var subInfoNeedDownload = make([]HdListItem, 0)
	// 有那些 Eps 需要下载的，按 SxEx 反回 epsKey
	for epsKey, epsInfo := range seriesInfo.NeedDlEpsKeyList {
		// 从一堆字幕里面找合适的
		value, ok := allSubDict[epsKey]
		// 是否有
		if ok == true && len(value) > 0 {
			value[0].Season = epsInfo.Season
			value[0].Episode = epsInfo.Episode
			subInfoNeedDownload = append(subInfoNeedDownload, value[0])
		} else {
			s.log.Infoln("SubHD Not Find Sub can be download", epsInfo.Title, epsInfo.Season, epsInfo.Episode)
		}
	}
	// 全季的字幕列表，也拼进去，后面进行下载
	for _, infos := range oneSeasonSubDict {
		subInfoNeedDownload = append(subInfoNeedDownload, infos[0])
	}

	// 返回前，需要把每一个 Eps 的 Season Episode 信息填充到每个 SubInfo 中
	return subInfoNeedDownload
}

// step0 找到这个影片的详情列表
func (s Supplier) step0(browser *rod.Browser, keyword string) (string, error) {
	var err error
	defer func() {
		if err != nil {
			notify_center.Notify.Add("subhd_step0", err.Error())
		}
	}()

	result, page, err := s.httpGetFromBrowser(browser, fmt.Sprintf(common.SubSubHDSearchUrl, url.QueryEscape(keyword)))
	if err != nil {
		return "", err
	}
	defer func() {
		_ = page.Close()
	}()
	// 是否有查找到的结果，至少要有结果。根据这里这样下面才能判断是分析失效了，还是就是没有结果而已
	re := regexp.MustCompile(`共\s*(\d+)\s*条`)
	matched := re.FindAllStringSubmatch(result, -1)
	if len(matched) < 1 {
		return "", common.SubHDStep0SubCountElementNotFound
	}
	subCount, err := decode.GetNumber2int(matched[0][0])
	if err != nil {
		return "", err
	}
	// 如果所搜没有找到字幕，就要返回
	if subCount < 1 {
		return "", nil
	}
	// 这里是确认能继续分析的详细连接
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(result))
	if err != nil {
		return "", err
	}
	imgSelection := doc.Find("img.rounded-start")
	_, ok := imgSelection.Attr("src")
	if ok == true {

		if len(imgSelection.Nodes) < 1 {
			return "", common.SubHDStep0ImgParentLessThan1
		}
		step1Url := ""
		if imgSelection.Nodes[0].Parent.Data == "a" {
			// 第一个父级是不是超链接
			for _, attribute := range imgSelection.Nodes[0].Parent.Attr {
				if attribute.Key == "href" {
					step1Url = attribute.Val
					break
				}
			}
		} else if imgSelection.Nodes[0].Parent.Parent.Data == "a" {
			// 第二个父级是不是超链接
			for _, attribute := range imgSelection.Nodes[0].Parent.Parent.Attr {
				if attribute.Key == "href" {
					step1Url = attribute.Val
					break
				}
			}
		}
		if step1Url == "" {
			return "", common.SubHDStep0HrefIsNull
		}
		return step1Url, nil
	} else {
		return "", common.SubHDStep0HrefIsNull
	}
}

// step1 获取影片的详情字幕列表
func (s Supplier) step1(browser *rod.Browser, detailPageUrl string, isMovieOrSeries bool) ([]HdListItem, error) {
	var err error
	defer func() {
		if err != nil {
			notify_center.Notify.Add("subhd_step1", err.Error())
		}
	}()
	detailPageUrl = pkg.AddBaseUrl(common.SubSubHDRootUrl, detailPageUrl)
	result, page, err := s.httpGetFromBrowser(browser, detailPageUrl)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = page.Close()
	}()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(result))
	if err != nil {
		return nil, err
	}
	var lists []HdListItem

	const subTableKeyword = ".pt-2"
	const oneSubTrTitleKeyword = "a.link-dark"
	const oneSubTrDownloadCountKeyword = "div.px-3"
	const oneSubLangAndTypeKeyword = ".text-secondary"

	doc.Find(subTableKeyword).EachWithBreak(func(i int, tr *goquery.Selection) bool {
		if tr.Find(oneSubTrTitleKeyword).Size() == 0 {
			return true
		}
		// 文件的下载页面，还需要分析
		downUrl, exists := tr.Find(oneSubTrTitleKeyword).Eq(0).Attr("href")
		if !exists {
			return true
		}
		// 文件名
		title := strings.TrimSpace(tr.Find(oneSubTrTitleKeyword).Text())
		// 字幕类型
		insideSubType := tr.Find(oneSubLangAndTypeKeyword).Text()
		if sub_helper.IsSubTypeWanted(insideSubType) == false {
			return true
		}
		// 下载的次数
		downCount, err := decode.GetNumber2int(tr.Find(oneSubTrDownloadCountKeyword).Eq(1).Text())
		if err != nil {
			return true
		}

		listItem := HdListItem{}
		listItem.Url = downUrl
		listItem.BaseUrl = common.SubSubHDRootUrl
		listItem.Title = title
		listItem.DownCount = downCount

		// 电影，就需要第一个
		// 连续剧，需要多个
		if isMovieOrSeries == true {

			if len(lists) >= s.topic {
				return false
			}
		}
		lists = append(lists, listItem)
		return true
	})

	return lists, nil
}

// step2Ex 下载字幕 过防水墙
func (s Supplier) step2Ex(browser *rod.Browser, subDownloadPageUrl string) (*HdContent, error) {
	var err error
	defer func() {
		if err != nil {
			notify_center.Notify.Add("subhd_step2Ex", err.Error())
		}
	}()
	subDownloadPageUrl = pkg.AddBaseUrl(common.SubSubHDRootUrl, subDownloadPageUrl)

	pageString, page, err := s.httpGetFromBrowser(browser, subDownloadPageUrl)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = page.Close()
	}()

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(pageString))
	if err != nil {
		return nil, err
	}
	// 是否有腾讯的防水墙
	hasWaterWall := true
	waterWall := doc.Find(TCode)
	if len(waterWall.Nodes) < 1 {
		hasWaterWall = false
	}
	hasDownBtn, BtnElemenString := s.JugDownloadBtn(doc)

	if hasWaterWall == false && hasDownBtn == false {
		// 都没有，则返回故障，无法下载
		return nil, common.SubHDStep2ExCannotFindDownloadBtn
	}
	// 下载字幕
	content, err := s.downloadSubFile(browser, page, hasWaterWall, BtnElemenString)
	if err != nil {
		return nil, err
	}

	return content, nil
}

func (s Supplier) JugDownloadBtn(doc *goquery.Document) (bool, string) {

	const btnDown0 = "#down"
	const btnDown1 = "button.down"
	// 是否有下载按钮
	hasDownBtn := true
	downBtn := doc.Find(btnDown0)
	if len(downBtn.Nodes) < 1 {
		hasDownBtn = false
	} else {
		return true, btnDown0
	}
	// 另一种是否有下载按钮的判断
	if hasDownBtn == false {
		downBtn = doc.Find(btnDown1)
		if len(downBtn.Nodes) < 1 {
			hasDownBtn = false
		} else {
			hasDownBtn = true
		}
	}
	return hasDownBtn, btnDown1
}

func (s Supplier) downloadSubFile(browser *rod.Browser, page *rod.Page, hasWaterWall bool, btnElemenString string) (*HdContent, error) {
	var err error
	fileName := ""
	fileByte := []byte{0}
	err = rod.Try(func() {
		tmpDir := filepath.Join(os.TempDir(), "rod", "downloads")
		wait := browser.WaitDownload(tmpDir)
		getDownloadFile := func() ([]byte, string, error) {
			info := wait()
			downloadPath := filepath.Join(tmpDir, info.GUID)
			defer func() { _ = os.Remove(downloadPath) }()
			b, err := ioutil.ReadFile(downloadPath)
			if err != nil {
				return nil, "", err
			}
			return b, info.SuggestedFilename, nil
		}

		// 点击下载按钮
		//var el *rod.Element
		if hasWaterWall == true {
			page.MustElement(TCode).MustClick()
		} else {
			page.MustElement(btnElemenString).MustClick()
		}
		// 找到遮挡的信息块，尝试移除
		//if err != nil {
		//if strings.Contains(err.Error(), "element covered by") == true {
		//	println("11")
		//	var eel *rod.ErrCovered
		//	if errors.As(err, &eel) == true {
		//		eel.MustRemove()
		//		err = el.Click(proto.InputMouseButtonLeft)
		//		if err != nil {
		//			print(123)
		//		}
		//	}
		//}
		//}
		// 过墙
		if hasWaterWall == true {
			s.passWaterWall(page)
		}
		fileByte, fileName, err = getDownloadFile()
		if err != nil {
			panic(err)
		}
	})
	if err != nil {
		return nil, err
	}

	var hdContent HdContent
	hdContent.Filename = fileName
	hdContent.Ext = filepath.Ext(fileName)
	hdContent.Data = fileByte

	return &hdContent, nil
}

func (s Supplier) passWaterWall(page *rod.Page) {
	//等待驗證碼窗體載入
	page.MustElement("#tcaptcha_iframe").MustWaitLoad()
	//進入到iframe
	iframe := page.MustElement("#tcaptcha_iframe").MustFrame()
	//等待拖動條加載, 延遲500秒檢測變化, 以確認加載完畢
	iframe.MustElement("#tcaptcha_drag_button").MustWaitStable()
	//等待缺口圖像載入
	slideBgEl := iframe.MustElement("#slideBg").MustWaitLoad()
	slideBgEl = slideBgEl.MustWaitStable()
	//取得帶缺口圖像
	shadowbg := slideBgEl.MustResource()
	// 取得原始圖像
	src := slideBgEl.MustProperty("src")
	fullbg, _, err := pkg.DownFile(strings.Replace(src.String(), "img_index=1", "img_index=0", 1))
	if err != nil {
		panic(err)
	}
	//取得img展示的真實尺寸
	shape, err := slideBgEl.Shape()
	if err != nil {
		panic(err)
	}
	bgbox := shape.Box()
	height, width := uint(math.Round(bgbox.Height)), uint(math.Round(bgbox.Width))
	//裁剪圖像
	shadowbgImg, _ := jpeg.Decode(bytes.NewReader(shadowbg))
	shadowbgImg = resize.Resize(width, height, shadowbgImg, resize.Lanczos3)
	fullbgImg, _ := jpeg.Decode(bytes.NewReader(fullbg))
	fullbgImg = resize.Resize(width, height, fullbgImg, resize.Lanczos3)

	//啓始left，排除干擾部份，所以右移10個像素
	left := fullbgImg.Bounds().Min.X + 10
	//啓始top, 排除干擾部份, 所以下移10個像素
	top := fullbgImg.Bounds().Min.Y + 10
	//最大left, 排除干擾部份, 所以左移10個像素
	maxleft := fullbgImg.Bounds().Max.X - 10
	//最大top, 排除干擾部份, 所以上移10個像素
	maxtop := fullbgImg.Bounds().Max.Y - 10
	//rgb比较阈值, 超出此阈值及代表找到缺口位置
	threshold := 20
	//缺口偏移, 拖動按鈕初始會偏移27.5
	distance := -27.5
	//取絕對值方法
	abs := func(n int) int {
		if n < 0 {
			return -n
		}
		return n
	}
search:
	for i := left; i <= maxleft; i++ {
		for j := top; j <= maxtop; j++ {
			colorAR, colorAG, colorAB, _ := fullbgImg.At(i, j).RGBA()
			colorBR, colorBG, colorBB, _ := shadowbgImg.At(i, j).RGBA()
			colorAR, colorAG, colorAB = colorAR>>8, colorAG>>8, colorAB>>8
			colorBR, colorBG, colorBB = colorBR>>8, colorBG>>8, colorBB>>8
			if abs(int(colorAR)-int(colorBR)) > threshold ||
				abs(int(colorAG)-int(colorBG)) > threshold ||
				abs(int(colorAB)-int(colorBB)) > threshold {
				distance += float64(i)
				s.log.Debug("對比完畢, 偏移量:", distance)
				break search
			}
		}
	}
	//獲取拖動按鈕形狀
	dragBtnBox := iframe.MustElement("#tcaptcha_drag_thumb").MustShape().Box()
	//启用滑鼠功能
	mouse := page.Mouse
	//模擬滑鼠移動至拖動按鈕處, 右移3的原因: 拖動按鈕比滑塊圖大3個像素
	mouse.MustMove(dragBtnBox.X+3, dragBtnBox.Y+(dragBtnBox.Height/2))
	//按下滑鼠左鍵
	mouse.MustDown("left")
	//開始拖動
	err = mouse.Move(dragBtnBox.X+distance, dragBtnBox.Y+(dragBtnBox.Height/2), 20)
	if err != nil {
		s.log.Errorln("mouse.Move", err)
	}
	//鬆開滑鼠左鍵, 拖动完毕
	mouse.MustUp("left")

	if s.reqParam.DebugMode == true {
		//截圖保存
		nowProcessRoot, err := pkg.GetDebugFolder()
		if err == nil {
			page.MustScreenshot(path.Join(nowProcessRoot, "result.png"))
		} else {
			s.log.Errorln("model.GetDebugFolder", err)
		}
	}
}

func (s Supplier) httpGetFromBrowser(browser *rod.Browser, inputUrl string) (string, *rod.Page, error) {

	page, err := rod_helper.NewPageNavigate(browser, inputUrl, s.tt, 5)
	if err != nil {
		return "", nil, err
	}
	pageString, err := page.HTML()
	if err != nil {
		return "", nil, err
	}
	// 每次搜索间隔在 30-40s
	time.Sleep(pkg.RandomSecondDuration(5, 10))

	return pageString, page, nil
}

type HdListItem struct {
	Url        string `json:"url"`
	BaseUrl    string `json:"baseUrl"`
	Title      string `json:"title"`
	Ext        string `json:"ext"`
	AuthorInfo string `json:"authorInfo"`
	Lang       string `json:"lang"`
	Rate       string `json:"rate"`
	DownCount  int    `json:"downCount"`
	Season     int    // 第几季，默认-1
	Episode    int    // 第几集，默认-1
}

type HdContent struct {
	Filename string `json:"filename"`
	Ext      string `json:"ext"`
	Data     []byte `json:"data"`
}

const TCode = "#TencentCaptcha"
