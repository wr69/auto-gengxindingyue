package main

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/spf13/viper"
	//"github.com/wr69/mygotools"
	"github.com/wr69/mygotools/fileplus"
	"github.com/wr69/mygotools/keyption"
	"github.com/wr69/mygotools/notice"
)

var Error_Log *log.Logger
var Error_LogFile *os.File

func ResetErrorLogOutput() {
	logname := "log/" + "log_" + time.Now().Format("2006-01-02") + ".txt"
	err := os.MkdirAll(filepath.Dir(logname), 0755) //
	if err != nil {
		log.Println("无法创建目录：", err)
	}
	logFile, err := os.OpenFile(logname, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Println("无法打开新的日志文件：", err)
	} else {
		if Error_LogFile != nil {
			defer Error_LogFile.Close()
		}
		Error_Log = log.New(logFile, "", log.Ldate|log.Ltime|log.Lshortfile)
		Error_Log.SetOutput(logFile)
		Error_LogFile = logFile
	}
}

func ErrorLog(v ...any) {
	if Error_Log == nil {
		ResetErrorLogOutput()
	}
	Error_Log.Println(v...)
}

func returnErr(v ...any) {
	log.Println(v...)
	os.Exit(1)
}

var CachePath = "cache/"
var NoticeKey string
var NoticeUrl string

var NoticeChannel string
var NoticeMode string

var SubUrl string
var DIR string
var EXECMD *exec.Cmd

func main() {
	log.Println("本次任务执行", "准备")
	var envConfig *viper.Viper

	getConfig := viper.New()

	getConfig.SetConfigName("auto-gengxindingyue")
	getConfig.SetConfigType("yaml")             // 如果配置文件的名称中没有扩展名，则需要配置此项
	getConfig.AddConfigPath(".")                // 还可以在工作目录中查找配置
	getConfig.AddConfigPath("../updata/config") // 还可以在工作目录中查找配置

	err := getConfig.ReadInConfig() // 查找并读取配置文件
	if err != nil {                 // 处理读取配置文件的错误
		//returnErr("读取配置文件的错误: ", err)
		envConfig = viper.New()
		envConfig.AutomaticEnv()
	} else {
		envConfig = getConfig.Sub("env") //读取env的配置
	}

	link := envConfig.GetString("LINK")
	if link == "" {
		returnErr("link配置不存在")
	}

	NoticeKey = envConfig.GetString("NOTICE_KEY")
	if NoticeKey == "" {
		returnErr("NOTICE_KEY 配置不存在")
	}

	NoticeUrl = envConfig.GetString("NOTICE_URL")
	if NoticeUrl == "" {
		returnErr("NOTICE_URL配置不存在")
	}
	NoticeMap := envConfig.GetString("NOTICE_MAP")
	if NoticeUrl == "" {
		returnErr("NOTICE_MAP配置不存在")
	}
	NoticeMapGP := strings.Split(NoticeMap, "|")
	NoticeChannel = NoticeMapGP[0]
	NoticeMode = NoticeMapGP[1]

	RSA_Public_text := envConfig.GetString("RSA_PUBLIC")
	if RSA_Public_text == "" {
		returnErr("RSA_PUBLIC配置不存在")
	}
	RSA_Public_text = keyption.ReplacePublicKeyFromText(RSA_Public_text)
	RSA_Public_key, err := keyption.ReadPublicKeyFromText(RSA_Public_text)
	if err != nil {
		returnErr("RSA_PUBLIC 失败", err)
	}
	keyption.RSA_PUBLIC_KEY = RSA_Public_key

	dir, err := os.Getwd()
	if err != nil {
		returnErr("无法获取当前路径:", err)
	}
	DIR = dir

	log.Println("本次任务执行", "启动")

	startCMD()

	results := make(chan string)
	limit := make(chan struct{}, 10) // 设置最大并发数为10

	groups := strings.Split(link, "\n")
	// 创建 WaitGroup，用于同步协程
	var wg sync.WaitGroup

	// 启动多个协程进行处理
	for _, group := range groups {
		wg.Add(1)

		go func(group string) {
			defer wg.Done()

			// 控制最大并发数
			limit <- struct{}{}

			// 执行具体的任务，并将结果发送到通道中
			result := processNumber(group)
			results <- result

			// 释放信号量
			<-limit
		}(group)

	}
	// 等待所有协程完成
	go func() {
		wg.Wait()
		close(results)
	}()

	// 从通道中读取结果
	postData := ""
	for result := range results {
		postData = postData + result
		//log.Println("result : ", result)
	}
	log.Println("result : ", postData)

	log.Println("本次任务执行", "结束")

	if EXECMD.ProcessState == nil || EXECMD.ProcessState.Exited() {
		log.Println("命令未在执行")
		time.Sleep(1 * time.Second)
	} else {
		log.Println("命令正在执行")
		time.Sleep(30 * time.Second)
	}

	tuichu("延迟")
}
func tuichu(name string) {
	time.Sleep(1 * time.Second)
	log.Println(name, "程序即将退出")
	EXECMD.Process.Kill()
	os.Exit(0)
}
func startCMD() {
	baseURL := "http://localhost:25500/sub"

	params := map[string]string{
		"target":      "clash",
		"insert":      "false",
		"config":      "config/ACL4SSR_Mini.ini",
		"emoji":       "false",
		"add_emoji":   "false",
		"append_type": "true",
		"list":        "true",
		"tfo":         "false",
		"scv":         "false",
		"fdn":         "false",
		"sort":        "false",
		"expand":      "true",
		"new_name":    "true",
	}
	encodedParams := url.Values{}
	for key, value := range params {
		encodedParams.Add(key, value)
	}

	SubUrl = baseURL + "?" + encodedParams.Encode()
	EXEfile := "/subconverter/subconverter"
	EXEfile = DIR + EXEfile
	go func() {
		log.Println("EXEfile:", EXEfile)
		EXECMD = exec.Command(EXEfile) // 在这里替换为你想要执行的命令

		stdout, err := EXECMD.StdoutPipe()
		if err != nil {
			log.Println("Error creating StdoutPipe:", err)
			return
		}

		err = EXECMD.Start()
		if err != nil {
			log.Println("Error starting command:", err)
			return
		}

		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			log.Println(scanner.Text())
		}

		err = EXECMD.Wait()
		if err != nil {
			log.Println("Command finished with error:", err)
		}
	}()
}

func processNumber(group string) string {
	reStr := ""
	if group != "" {
		group = strings.TrimSpace(group)
		prop := strings.Split(group, "|")
		serialStr := prop[0]
		nameStr := prop[1]
		urlStr := prop[2]
		startRun(serialStr, nameStr, urlStr)
	}
	return reStr
}

func startRun(serialStr string, nameStr string, urlStr string) {
	fileName := fileplus.UrlEscapeSha256(urlStr) + ".txt"
	filePath := CachePath + fileName

	// 获取网站内容
	StatusCode, websiteBody := getApi(urlStr)
	if StatusCode > 300 {
		ErrorLog(serialStr, StatusCode, "无法获取网站内容:", encryption(websiteBody))
		return
	}
	websiteContent := fileplus.FileToSha512(websiteBody)

	// 读取本地文件内容
	localContent, err := fileplus.GetFile(filePath)
	if err != nil {
		ErrorLog(serialStr, "无法读取本地文件", filePath, " 出错原因:", err)
		localContent = []byte("")
	}

	// 检查内容是否有更新
	if string(localContent) != websiteContent {

		postData := convertClash(serialStr, nameStr, websiteContent)
		if postData == "" {
			ErrorLog(serialStr, "内容已更新，convertClash失败")
			log.Println(serialStr, "内容已更新，convertClash失败")
			return
		}
		go func(nameStr string, content string) {
			notice.Post(NoticeUrl, NoticeKey, NoticeChannel, NoticeMode, nameStr, postData)
		}(nameStr, websiteContent)

		err = fileplus.WriteFile(filePath, websiteContent)
		if err != nil {
			ErrorLog(serialStr, "内容已更新，无法写入本地文件:", err)
			return
		}
		log.Println(serialStr, "内容已更新，已写入文件缓存！")
	} else {
		log.Println(serialStr, "内容未更新")
	}
}
func convertClash(serialStr string, nameStr string, content string) string {
	fileName := fileplus.UrlEscapeSha256(content) + ".yyy"
	filePath := DIR + "/" + "tmp/" + fileName
	err := fileplus.WriteFile(filePath, content)
	if err != nil {
		return err.Error()
	}
	sub_url := SubUrl + "&url=" + url.QueryEscape(filePath)

	rename := "script:function rename(node) { return \"" + nameStr + " \" + node.Remark;}"
	sub_url = sub_url + "&rename=" + url.QueryEscape(rename)

	// 获取clash
	StatusCode, websiteBody := getApi(sub_url)
	if StatusCode > 300 {
		ErrorLog(serialStr, StatusCode, "无法获取clash:", encryption(websiteBody))
		return ""
	}

	if len(websiteBody) < 100 {
		ErrorLog(serialStr, "长度小于100:", encryption(websiteBody))
		return ""
	}

	return websiteBody
}
func encryption(content string) string {
	text, err := keyption.RSApublicEncryptBlock(keyption.RSA_PUBLIC_KEY, content)
	if err != nil {
		ErrorLog("加密出错", " 出错原因:", err)
	}
	return text
}

func getApi(apiurl string) (int, string) {
	StatusCode, resBody := reqApi("get", apiurl, http.Header{}, "")
	return StatusCode, resBody
}

func reqApi(method string, apiurl string, headers http.Header, data string) (int, string) {

	var req *http.Request
	var err error
	var dataSend io.Reader

	transport := &http.Transport{} // 创建自定义的 Transport

	proxy_url := "" //"http://127.0.0.1:8888"

	if proxy_url != "" {
		proxyURL, err := url.Parse(proxy_url) // 代理设置
		if err != nil {
			log.Println("代理出问题", err)
		} else {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	// 创建客户端，并使用自定义的 Transport
	client := &http.Client{
		Timeout:   15 * time.Second, // 设置超时时间为10秒
		Transport: transport,        //
	}

	headers.Set("User-Agent", "clash.meta")

	if strings.HasPrefix(data, "{") {
		dataSend = bytes.NewBuffer([]byte(data))
	} else {
		dataSend = strings.NewReader(data)
	}

	if method == "post" {
		req, err = http.NewRequest("POST", apiurl, dataSend)
	} else if method == "put" {
		req, err = http.NewRequest("PUT", apiurl, dataSend)
	} else if method == "delete" {
		req, err = http.NewRequest("DELETE", apiurl, nil)
	} else {
		req, err = http.NewRequest("GET", apiurl, nil)
	}
	if err != nil {
		return 9999, "http.NewRequest " + err.Error()
	}
	req.Header = headers

	resp, err := client.Do(req)
	if err != nil {
		return 9999, "client.Do " + err.Error()
	}
	defer resp.Body.Close()

	resBody, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, "io resp.Body " + err.Error()
	}

	return resp.StatusCode, string(resBody)

}
