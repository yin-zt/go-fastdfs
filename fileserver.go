package main

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"flag"
	"fmt"
	"github.com/astaxie/beego/httplib"
	"github.com/deckarep/golang-set"
	_ "github.com/eventials/go-tus"
	"github.com/json-iterator/go"
	log "github.com/sjqzhang/seelog"
	"github.com/sjqzhang/tusd"
	"github.com/sjqzhang/tusd/filestore"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"io"
	"io/ioutil"
	slog "log"
	random "math/rand"
	"mime/multipart"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/smtp"
	"net/url"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var staticHandler http.Handler
var json = jsoniter.ConfigCompatibleWithStandardLibrary
var server *Server
var logacc log.LoggerInterface
var FOLDERS = []string{DATA_DIR, STORE_DIR, CONF_DIR}
var CONST_QUEUE_SIZE = 10000
var (
	FileName                    string
	ptr                         unsafe.Pointer
	DOCKER_DIR                  = ""
	STORE_DIR                   = STORE_DIR_NAME
	CONF_DIR                    = CONF_DIR_NAME
	LOG_DIR                     = LOG_DIR_NAME
	DATA_DIR                    = DATA_DIR_NAME
	LARGE_DIR_NAME              = "haystack"
	LARGE_DIR                   = STORE_DIR + "/haystack"
	CONST_LEVELDB_FILE_NAME     = DATA_DIR + "/fileserver.db"
	CONST_LOG_LEVELDB_FILE_NAME = DATA_DIR + "/log.db"
	CONST_STAT_FILE_NAME        = DATA_DIR + "/stat.json"
	CONST_CONF_FILE_NAME        = CONF_DIR + "/cfg.json"
	logConfigStr                = `
<seelog type="asynctimer" asyncinterval="1000" minlevel="trace" maxlevel="error">  
	<outputs formatid="common">  
		<buffered formatid="common" size="1048576" flushperiod="1000">  
			<rollingfile type="size" filename="{DOCKER_DIR}log/fileserver.log" maxsize="104857600" maxrolls="10"/>  
		</buffered>
	</outputs>  	  
	 <formats>
		 <format id="common" format="%Date %Time [%LEV] [%File:%Line] [%Func] %Msg%n" />  
	 </formats>  
</seelog>
`
	logAccessConfigStr = `
<seelog type="asynctimer" asyncinterval="1000" minlevel="trace" maxlevel="error">  
	<outputs formatid="common">  
		<buffered formatid="common" size="1048576" flushperiod="1000">  
			<rollingfile type="size" filename="{DOCKER_DIR}log/access.log" maxsize="104857600" maxrolls="10"/>  
		</buffered>
	</outputs>  	  
	 <formats>
		 <format id="common" format="%Date %Time [%LEV] [%File:%Line] [%Func] %Msg%n" />  
	 </formats>  
</seelog>
`
)

const (
	STORE_DIR_NAME                 = "files"
	LOG_DIR_NAME                   = "log"
	DATA_DIR_NAME                  = "data"
	CONF_DIR_NAME                  = "conf"
	CONST_STAT_FILE_COUNT_KEY      = "fileCount"
	CONST_BIG_UPLOAD_PATH_SUFFIX   = "/big/upload/"
	CONST_STAT_FILE_TOTAL_SIZE_KEY = "totalSize"
	CONST_Md5_ERROR_FILE_NAME      = "errors.md5"
	CONST_Md5_QUEUE_FILE_NAME      = "queue.md5"
	CONST_FILE_Md5_FILE_NAME       = "files.md5"
	CONST_REMOME_Md5_FILE_NAME     = "removes.md5"
	CONST_SMALL_FILE_SIZE          = 1024 * 1024
	CONST_MESSAGE_CLUSTER_IP       = "Can only be called by the cluster ip,current ip:%s"
	cfgJson                        = `{
	"绑定端号": "端口",
	"addr": ":8080",
	"PeerID": "集群内唯一,请使用0-9的单字符，默认自动生成",
	"peer_id": "%s",
	"本主机地址": "本机http地址,默认自动生成，必段为内网，自动生成不为内网请自行修改，下同",
	"host": "%s",
	"集群": "集群列表,注意为了高可用，IP必须不能是同一个,同一不会自动备份，且不能为127.0.0.1,且必须为内网IP，默认自动生成",
	"peers": ["%s"],
	"组号": "用于区别不同的集群(上传或下载)与support_group_upload配合使用,带在下载路径中",
	"group": "group1",
	"是否合并小文件": "默认不合并,合并可以解决inode不够用的情况（当前对于小于1M文件）进行合并",
	"enable_merge_small_file": false,
	"重试同步失败文件的时间": "单位秒",
	"refresh_interval": 1800,
	"是否自动重命名": "默认不自动重命名,使用原文件名",
	"rename_file": false,
	"是否支持web上传,方便调试": "默认支持web上传",
	"enable_web_upload": true,
	"是否支持非日期路径": "默认支持非日期路径,也即支持自定义路径,需要上传文件时指定path",
	"enable_custom_path": true,
	"下载域名": "用于外网下载文件的域名,不包含http://",
	"download_domain": "",
	"场景列表": "当设定后，用户指的场景必项在列表中，默认不做限制",
	"scenes": [],
	"默认场景": "默认default",
	"default_scene": "default",
	"是否显示目录": "默认显示,方便调试用,上线时请关闭",
	"show_dir": true,
	"邮件配置": "",
	"mail": {
		"user": "abc@163.com",
		"password": "abc",
		"host": "smtp.163.com:25"
	},
	"告警接收邮件列表": "接收人数组",
	"alram_receivers": [],
	"告警接收URL": "方法post,参数:subjet,message",
	"alarm_url": "",
	"下载是否需带token": "真假",
	"download_use_token": false,
	"下载token过期时间": "单位秒",
	"download_token_expire": 600,
	"是否自动修复": "在超过1亿文件时出现性能问题，取消此选项，请手动按天同步，请查看FAQ",
	"auto_repair": true,
	"文件去重算法md5可能存在冲突，默认md5": "sha1|md5",
	"file_sum_arithmetic": "md5",
	"是否支持按组（集群）管理,主要用途是Nginx支持多集群": "默认不支持,不支持时路径为http://10.1.5.4:8080/action,支持时为http://10.1.5.4:8080/group(配置中的group参数)/action,action为动作名，如status,delete,sync等",
	"support_group_manage": false,
	"管理ip列表": "用于管理集的ip白名单,",
	"admin_ips": ["127.0.0.1"]
}
	`
)

type Common struct {
}
type Server struct {
	ldb            *leveldb.DB
	logDB          *leveldb.DB
	util           *Common
	statMap        *CommonMap
	sumMap         *CommonMap //map[string]mapset.Set
	queueToPeers   chan FileInfo
	queueFromPeers chan FileInfo
	queueFileLog   chan *FileLog
	lockMap        *CommonMap
	curDate        string
	host           string
}
type FileInfo struct {
	Name      string   `json:"name"`
	ReName    string   `json:"rename"`
	Path      string   `json:"path"`
	Md5       string   `json:"md5"`
	Size      int64    `json:"size"`
	Peers     []string `json:"peers"`
	Scene     string   `json:"scene"`
	TimeStamp int64    `json:"timeStamp"`
	OffSet    int64    `json:"offset"`
}
type FileLog struct {
	FileInfo *FileInfo
	FileName string
}
type JsonResult struct {
	Message string      `json:"message"`
	Status  string      `json:"status"`
	Data    interface{} `json:"data"`
}
type FileResult struct {
	Url    string `json:"url"`
	Md5    string `json:"md5"`
	Path   string `json:"path"`
	Domain string `json:"domain"`
	Scene  string `json:"scene"`
	//Just for Compatibility
	Scenes  string `json:"scenes"`
	Retmsg  string `json:"retmsg"`
	Retcode int    `json:"retcode"`
	Src     string `json:"src"`
}
type Mail struct {
	User     string `json:"user"`
	Password string `json:"password"`
	Host     string `json:"host"`
}
type StatDateFileInfo struct {
	Date      string `json:"date"`
	TotalSize int64  `json:"totalSize"`
	FileCount int64  `json:"fileCount"`
}
type GloablConfig struct {
	Addr                 string   `json:"addr"`
	Peers                []string `json:"peers"`
	Group                string   `json:"group"`
	RenameFile           bool     `json:"rename_file"`
	ShowDir              bool     `json:"show_dir"`
	RefreshInterval      int      `json:"refresh_interval"`
	EnableWebUpload      bool     `json:"enable_web_upload"`
	DownloadDomain       string   `json:"download_domain"`
	EnableCustomPath     bool     `json:"enable_custom_path"`
	Scenes               []string `json:"scenes"`
	AlramReceivers       []string `json:"alram_receivers"`
	DefaultScene         string   `json:"default_scene"`
	Mail                 Mail     `json:"mail"`
	AlarmUrl             string   `json:"alarm_url"`
	DownloadUseToken     bool     `json:"download_use_token"`
	DownloadTokenExpire  int      `json:"download_token_expire"`
	QueueSize            int      `json:"queue_size"`
	AutoRepair           bool     `json:"auto_repair"`
	Host                 string   `json:"host"`
	FileSumArithmetic    string   `json:"file_sum_arithmetic"`
	PeerId               string   `json:"peer_id"`
	SupportGroupManage   bool     `json:"support_group_manage"`
	AdminIps             []string `json:"admin_ips"`
	EnableMergeSmallFile bool     `json:"enable_merge_small_file"`
}

func NewServer() *Server {
	var (
		server *Server
		err    error
	)
	server = &Server{
		util:           &Common{},
		statMap:        NewCommonMap(0),
		lockMap:        NewCommonMap(0),
		queueToPeers:   make(chan FileInfo, CONST_QUEUE_SIZE),
		queueFromPeers: make(chan FileInfo, CONST_QUEUE_SIZE),
		queueFileLog:   make(chan *FileLog, CONST_QUEUE_SIZE),
		sumMap:         NewCommonMap(365 * 3),
	}

	defaultTransport := &http.Transport{
		DisableKeepAlives:   true,
		Dial:                httplib.TimeoutDialer(time.Second*6, time.Second*60),
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
	}
	settins := httplib.BeegoHTTPSettings{
		UserAgent:        "Go-FastDFS",
		ConnectTimeout:   10 * time.Second,
		ReadWriteTimeout: 10 * time.Second,
		Gzip:             true,
		DumpBody:         true,
		Transport:        defaultTransport,
	}
	httplib.SetDefaultSetting(settins)
	server.statMap.Put(CONST_STAT_FILE_COUNT_KEY, int64(0))
	server.statMap.Put(CONST_STAT_FILE_TOTAL_SIZE_KEY, int64(0))
	server.statMap.Put(server.util.GetToDay()+"_"+CONST_STAT_FILE_COUNT_KEY, int64(0))
	server.statMap.Put(server.util.GetToDay()+"_"+CONST_STAT_FILE_TOTAL_SIZE_KEY, int64(0))
	server.curDate = server.util.GetToDay()
	server.ldb, err = leveldb.OpenFile(CONST_LEVELDB_FILE_NAME, nil)
	if err != nil {
		fmt.Println(err)
		panic(err)
		log.Error(err)
	}
	server.logDB, err = leveldb.OpenFile(CONST_LOG_LEVELDB_FILE_NAME, nil)
	if err != nil {
		fmt.Println(err)
		panic(err)
		log.Error(err)
	}
	return server
}

type CommonMap struct {
	sync.Mutex
	m map[string]interface{}
}

func NewCommonMap(size int) *CommonMap {
	if size > 0 {
		return &CommonMap{m: make(map[string]interface{}, size)}
	} else {
		return &CommonMap{m: make(map[string]interface{})}
	}
}
func (s *CommonMap) GetValue(k string) (interface{}, bool) {
	s.Lock()
	defer s.Unlock()
	v, ok := s.m[k]
	return v, ok
}
func (s *CommonMap) Put(k string, v interface{}) {
	s.Lock()
	defer s.Unlock()
	s.m[k] = v
}
func (s *CommonMap) LockKey(k string) {
	s.Lock()
	if v, ok := s.m[k]; ok {
		s.m[k+"_lock_"] = true
		s.Unlock()
		v.(*sync.Mutex).Lock()
	} else {
		s.m[k] = &sync.Mutex{}
		v = s.m[k]
		s.m[k+"_lock_"] = true
		s.Unlock()
		v.(*sync.Mutex).Lock()
	}
}
func (s *CommonMap) UnLockKey(k string) {
	s.Lock()
	if v, ok := s.m[k]; ok {
		v.(*sync.Mutex).Unlock()
		s.m[k+"_lock_"] = false
	}
	s.Unlock()
}
func (s *CommonMap) IsLock(k string) bool {
	s.Lock()
	if v, ok := s.m[k+"_lock_"]; ok {
		s.Unlock()
		return v.(bool)
	}
	s.Unlock()
	return false
}
func (s *CommonMap) Keys() []string {
	s.Lock()
	keys := make([]string, len(s.m))
	defer s.Unlock()
	for k, _ := range s.m {
		keys = append(keys, k)
	}
	return keys
}
func (s *CommonMap) Clear() {
	s.Lock()
	defer s.Unlock()
	s.m = make(map[string]interface{})
}
func (s *CommonMap) Remove(key string) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.m[key]; ok {
		delete(s.m, key)
	}
}
func (s *CommonMap) AddUniq(key string) {
	s.Lock()
	defer s.Unlock()
	if _, ok := s.m[key]; !ok {
		s.m[key] = nil
	}
}
func (s *CommonMap) AddCount(key string, count int) {
	s.Lock()
	defer s.Unlock()
	if _v, ok := s.m[key]; ok {
		v := _v.(int)
		v = v + count
		s.m[key] = v
	} else {
		s.m[key] = 1
	}
}
func (s *CommonMap) AddCountInt64(key string, count int64) {
	s.Lock()
	defer s.Unlock()
	if _v, ok := s.m[key]; ok {
		v := _v.(int64)
		v = v + count
		s.m[key] = v
	} else {
		s.m[key] = count
	}
}
func (s *CommonMap) Add(key string) {
	s.Lock()
	defer s.Unlock()
	if _v, ok := s.m[key]; ok {
		v := _v.(int)
		v = v + 1
		s.m[key] = v
	} else {
		s.m[key] = 1
	}
}
func (s *CommonMap) Zero() {
	s.Lock()
	defer s.Unlock()
	for k := range s.m {
		s.m[k] = 0
	}
}
func (s *CommonMap) Contains(i ...interface{}) bool {
	s.Lock()
	defer s.Unlock()
	for _, val := range i {
		if _, ok := s.m[val.(string)]; !ok {
			return false
		}
	}
	return true
}
func (s *CommonMap) Get() map[string]interface{} {
	s.Lock()
	defer s.Unlock()
	m := make(map[string]interface{})
	for k, v := range s.m {
		m[k] = v
	}
	return m
}
func Config() *GloablConfig {
	return (*GloablConfig)(atomic.LoadPointer(&ptr))
}
func ParseConfig(filePath string) {
	var (
		data []byte
	)
	if filePath == "" {
		data = []byte(strings.TrimSpace(cfgJson))
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			panic(fmt.Sprintln("open file path:", filePath, "error:", err))
		}
		defer file.Close()
		FileName = filePath
		data, err = ioutil.ReadAll(file)
		if err != nil {
			panic(fmt.Sprintln("file path:", filePath, " read all error:", err))
		}
	}
	var c GloablConfig
	if err := json.Unmarshal(data, &c); err != nil {
		panic(fmt.Sprintln("file path:", filePath, "json unmarshal error:", err))
	}
	log.Info(c)
	atomic.StorePointer(&ptr, unsafe.Pointer(&c))
	log.Info("config parse success")
}
func (this *Common) GetUUID() string {
	b := make([]byte, 48)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return ""
	}
	id := this.MD5(base64.URLEncoding.EncodeToString(b))
	return fmt.Sprintf("%s-%s-%s-%s-%s", id[0:8], id[8:12], id[12:16], id[16:20], id[20:])
}
func (this *Common) CopyFile(src, dst string) (int64, error) {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	if !sourceFileStat.Mode().IsRegular() {
		return 0, fmt.Errorf("%s is not a regular file", src)
	}
	source, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return 0, err
	}
	defer destination.Close()
	nBytes, err := io.Copy(destination, source)
	return nBytes, err
}
func (this *Common) RandInt(min, max int) int {
	return func(min, max int) int {
		r := random.New(random.NewSource(time.Now().UnixNano()))
		if min >= max {
			return max
		}
		return r.Intn(max-min) + min
	}(min, max)
}
func (this *Common) GetToDay() string {
	return time.Now().Format("20060102")
}
func (this *Common) UrlEncode(v interface{}) string {
	switch v.(type) {
	case string:
		m := make(map[string]string)
		m["name"] = v.(string)
		return strings.Replace(this.UrlEncodeFromMap(m), "name=", "", 1)
	case map[string]string:
		return this.UrlEncodeFromMap(v.(map[string]string))
	default:
		return fmt.Sprintf("%v", v)
	}
}
func (this *Common) UrlEncodeFromMap(m map[string]string) string {
	vv := url.Values{}
	for k, v := range m {
		vv.Add(k, v)
	}
	return vv.Encode()
}
func (this *Common) UrlDecodeToMap(body string) (map[string]string, error) {
	var (
		err error
		m   map[string]string
		v   url.Values
	)
	m = make(map[string]string)
	if v, err = url.ParseQuery(body); err != nil {
		return m, err
	}
	for _k, _v := range v {
		if len(_v) > 0 {
			m[_k] = _v[0]
		}
	}
	return m, nil
}
func (this *Common) GetDayFromTimeStamp(timeStamp int64) string {
	return time.Unix(timeStamp, 0).Format("20060102")
}
func (this *Common) StrToMapSet(str string, sep string) mapset.Set {
	result := mapset.NewSet()
	for _, v := range strings.Split(str, sep) {
		result.Add(v)
	}
	return result
}
func (this *Common) MapSetToStr(set mapset.Set, sep string) string {
	var (
		ret []string
	)
	for v := range set.Iter() {
		ret = append(ret, v.(string))
	}
	return strings.Join(ret, sep)
}
func (this *Common) GetPulicIP() string {
	var (
		err  error
		conn net.Conn
	)
	if conn, err = net.Dial("udp", "8.8.8.8:80"); err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().String()
	idx := strings.LastIndex(localAddr, ":")
	return localAddr[0:idx]
}
func (this *Common) MD5(str string) string {
	md := md5.New()
	md.Write([]byte(str))
	return fmt.Sprintf("%x", md.Sum(nil))
}
func (this *Common) GetFileMd5(file *os.File) string {
	file.Seek(0, 0)
	md5h := md5.New()
	io.Copy(md5h, file)
	sum := fmt.Sprintf("%x", md5h.Sum(nil))
	return sum
}
func (this *Common) GetFileSum(file *os.File, alg string) string {
	alg = strings.ToLower(alg)
	if alg == "sha1" {
		return this.GetFileSha1Sum(file)
	} else {
		return this.GetFileMd5(file)
	}
}
func (this *Common) GetFileSumByName(filepath string, alg string) (string, error) {
	var (
		err  error
		file *os.File
	)
	file, err = os.Open(filepath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	alg = strings.ToLower(alg)
	if alg == "sha1" {
		return this.GetFileSha1Sum(file), nil
	} else {
		return this.GetFileMd5(file), nil
	}
}
func (this *Common) GetFileSha1Sum(file *os.File) string {
	file.Seek(0, 0)
	md5h := sha1.New()
	io.Copy(md5h, file)
	sum := fmt.Sprintf("%x", md5h.Sum(nil))
	return sum
}
func (this *Common) WriteFileByOffSet(filepath string, offset int64, data []byte) (error) {
	var (
		err   error
		file  *os.File
		count int
	)
	file, err = os.OpenFile(filepath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	defer file.Close()
	count, err = file.WriteAt(data, offset)
	if err != nil {
		return err
	}
	if count != len(data) {
		return errors.New(fmt.Sprintf("write %s error", filepath))
	}
	return nil
}
func (this *Common) ReadFileByOffSet(filepath string, offset int64, length int) ([]byte, error) {
	var (
		err    error
		file   *os.File
		result []byte
		count  int
	)
	file, err = os.Open(filepath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result = make([]byte, length)
	count, err = file.ReadAt(result, offset)
	if err != nil {
		return nil, err
	}
	if count != length {
		return nil, errors.New("read error")
	}
	return result, nil
}
func (this *Common) Contains(obj interface{}, arrayobj interface{}) bool {
	targetValue := reflect.ValueOf(arrayobj)
	switch reflect.TypeOf(arrayobj).Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < targetValue.Len(); i++ {
			if targetValue.Index(i).Interface() == obj {
				return true
			}
		}
	case reflect.Map:
		if targetValue.MapIndex(reflect.ValueOf(obj)).IsValid() {
			return true
		}
	}
	return false
}
func (this *Common) FileExists(fileName string) bool {
	_, err := os.Stat(fileName)
	return err == nil
}
func (this *Common) WriteFile(path string, data string) bool {
	if err := ioutil.WriteFile(path, []byte(data), 0775); err == nil {
		return true
	} else {
		return false
	}
}
func (this *Common) WriteBinFile(path string, data []byte) bool {
	if err := ioutil.WriteFile(path, data, 0775); err == nil {
		return true
	} else {
		return false
	}
}
func (this *Common) IsExist(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}
func (this *Common) Match(matcher string, content string) []string {
	var result []string
	if reg, err := regexp.Compile(matcher); err == nil {
		result = reg.FindAllString(content, -1)
	}
	return result
}
func (this *Common) ReadBinFile(path string) ([]byte, error) {
	if this.IsExist(path) {
		fi, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer fi.Close()
		return ioutil.ReadAll(fi)
	} else {
		return nil, errors.New("not found")
	}
}
func (this *Common) RemoveEmptyDir(pathname string) {
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("postFileToPeer")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	handlefunc := func(file_path string, f os.FileInfo, err error) error {
		if f.IsDir() {
			files, _ := ioutil.ReadDir(file_path)
			if len(files) == 0 && file_path != pathname {
				os.Remove(file_path)
			}
		}
		return nil
	}
	fi, _ := os.Stat(pathname)
	if fi.IsDir() {
		filepath.Walk(pathname, handlefunc)
	}
}
func (this *Common) JsonEncodePretty(o interface{}) string {
	resp := ""
	switch o.(type) {
	case map[string]interface{}:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case map[string]string:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case []interface{}:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case []string:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	case string:
		resp = o.(string)
	default:
		if data, err := json.Marshal(o); err == nil {
			resp = string(data)
		}
	}
	var v interface{}
	if ok := json.Unmarshal([]byte(resp), &v); ok == nil {
		if buf, ok := json.MarshalIndent(v, "", "  "); ok == nil {
			resp = string(buf)
		}
	}
	return resp
}
func (this *Common) GetClientIp(r *http.Request) string {
	client_ip := ""
	headers := []string{"X_Forwarded_For", "X-Forwarded-For", "X-Real-Ip",
		"X_Real_Ip", "Remote_Addr", "Remote-Addr"}
	for _, v := range headers {
		if _v, ok := r.Header[v]; ok {
			if len(_v) > 0 {
				client_ip = _v[0]
				break
			}
		}
	}
	if client_ip == "" {
		clients := strings.Split(r.RemoteAddr, ":")
		client_ip = clients[0]
	}
	return client_ip
}
func (this *Server) BackUpMetaDataByDate(date string) {
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("BackUpMetaDataByDate")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	var (
		err          error
		keyPrefix    string
		msg          string
		name         string
		fileInfo     FileInfo
		logFileName  string
		fileLog      *os.File
		fileMeta     *os.File
		metaFileName string
	)
	logFileName = DATA_DIR + "/" + date + "/" + CONST_FILE_Md5_FILE_NAME
	this.lockMap.LockKey(logFileName)
	defer this.lockMap.UnLockKey(logFileName)
	metaFileName = DATA_DIR + "/" + date + "/" + "meta.data"
	os.MkdirAll(DATA_DIR+"/"+date, 0775)
	if this.util.IsExist(logFileName) {
		os.Remove(logFileName)
	}
	if this.util.IsExist(metaFileName) {
		os.Remove(metaFileName)
	}
	fileLog, err = os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0664)
	if err != nil {
		log.Error(err)
		return
	}
	defer fileLog.Close()
	fileMeta, err = os.OpenFile(metaFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0664)
	if err != nil {
		log.Error(err)
		return
	}
	defer fileMeta.Close()
	keyPrefix = "%s_%s_"
	keyPrefix = fmt.Sprintf(keyPrefix, date, CONST_FILE_Md5_FILE_NAME)
	iter := server.logDB.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
	defer iter.Release()
	for iter.Next() {
		if err = json.Unmarshal(iter.Value(), &fileInfo); err != nil {
			continue
		}
		name = fileInfo.Name
		if fileInfo.ReName != "" {
			name = fileInfo.ReName
		}
		msg = fmt.Sprintf("%s\t%s\n", fileInfo.Md5, string(iter.Value()))
		fileMeta.WriteString(msg)
		msg = fmt.Sprintf("%s\t%s\n", this.util.MD5(fileInfo.Path+"/"+name), string(iter.Value()))
		fileMeta.WriteString(msg)
		msg = fmt.Sprintf("%s|%d|%d|%s\n", fileInfo.Md5, fileInfo.Size, fileInfo.TimeStamp, fileInfo.Path+"/"+name)
		fileLog.WriteString(msg)
	}
}
func (this *Server) RepairStatByDate(date string) StatDateFileInfo {
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("RepairStatByDate")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	var (
		err       error
		keyPrefix string
		fileInfo  FileInfo
		fileCount int64
		fileSize  int64
		stat      StatDateFileInfo
	)
	keyPrefix = "%s_%s_"
	keyPrefix = fmt.Sprintf(keyPrefix, date, CONST_FILE_Md5_FILE_NAME)
	iter := server.logDB.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
	defer iter.Release()
	for iter.Next() {
		if err = json.Unmarshal(iter.Value(), &fileInfo); err != nil {
			continue
		}
		fileCount = fileCount + 1
		fileSize = fileSize + fileInfo.Size
	}
	this.statMap.Put(date+"_"+CONST_STAT_FILE_COUNT_KEY, fileCount)
	this.statMap.Put(date+"_"+CONST_STAT_FILE_TOTAL_SIZE_KEY, fileSize)
	this.SaveStat()
	stat.Date = date
	stat.FileCount = fileCount
	stat.TotalSize = fileSize
	return stat
}
func (this *Server) CheckFileExistByMd5(md5s string, fileInfo *FileInfo) bool {
	var (
		err    error
		info   *FileInfo
		fn     string
		name   string
		offset int64
		data   []byte
	)
	if info, err = this.GetFileInfoFromLevelDB(md5s); err != nil {
		return false
	}
	if info == nil || info.Md5 == "" {
		return false
	}
	fn = info.Name
	if info.ReName != "" {
		fn = info.ReName
	}
	if info.OffSet == -1 {
		if this.util.FileExists(DOCKER_DIR + info.Path + "/" + fn) {
			return true
		}
	} else { //small file
		if name, offset, _, err = this.ParseSmallFile(fn); err != nil {
			return false
		}
		if !this.util.FileExists(DOCKER_DIR + info.Path + "/" + name) {
			return false
		}
		if data, err = this.util.ReadFileByOffSet(DOCKER_DIR+info.Path+"/"+name, offset, 1); err != nil {
			return false
		}
		if data[0] == '1' {
			return true
		}
	}
	if info != nil && info.Md5 != "" {
		if fileInfo != nil {
			if fileInfo.Path != info.Path {
				return false
			}
		}
		return true
	} else {
		return false
	}
}
func (this *Server) ParseSmallFile(filename string) (string, int64, int, error) {
	var (
		err    error
		offset int64
		length int
	)
	err = errors.New("unvalid small file")
	if len(filename) < 3 {
		return filename, -1, -1, err
	}
	if strings.Contains(filename, "/") {
		filename = filename[strings.LastIndex(filename, "/")+1:]
	}
	pos := strings.Split(filename, ",")
	if len(pos) < 3 {
		return filename, -1, -1, err
	}
	offset, err = strconv.ParseInt(pos[1], 10, 64)
	if err != nil {
		return filename, -1, -1, err
	}
	if length, err = strconv.Atoi(pos[2]); err != nil {
		return filename, offset, -1, err
	}
	if length > CONST_SMALL_FILE_SIZE || offset < 0 {
		err = errors.New("invalid filesize or offset")
		return filename, -1, -1, err
	}
	return pos[0], offset, length, nil
}
func (this *Server) DownloadFromPeer(peer string, fileInfo *FileInfo) {
	var (
		err      error
		filename string
		fpath    string
		fi       os.FileInfo
		sum      string
		data     []byte
	)
	filename = fileInfo.Name
	if fileInfo.ReName != "" {
		filename = fileInfo.ReName
	}
	if this.CheckFileExistByMd5(fileInfo.Md5, fileInfo) {
		return
	}
	if _, err = os.Stat(fileInfo.Path); err != nil {
		os.MkdirAll(DOCKER_DIR+fileInfo.Path, 0775)
	}
	//fmt.Println("downloadFromPeer",fileInfo)
	p := strings.Replace(fileInfo.Path, STORE_DIR_NAME+"/", "", 1)
	//filename=this.util.UrlEncode(filename)
	req := httplib.Get(peer + "/" + Config().Group + "/" + p + "/" + filename)
	fpath = DOCKER_DIR + fileInfo.Path + "/" + filename
	timeout := fileInfo.Size/1024/1024/8 + 30
	req.SetTimeout(time.Second*5, time.Second*time.Duration(timeout))
	if fileInfo.OffSet != -1 { //small file download
		data, err = req.Bytes()
		if err != nil {
			log.Error(err)
			return
		}
		data2 := make([]byte, len(data)+1)
		data2[0] = '1'
		for i, v := range data {
			data2[i+1] = v
		}
		data = data2
		if int64(len(data)) != fileInfo.Size {
			log.Warn("file size is error")
			return
		}
		fpath = strings.Split(fpath, ",")[0]
		err = this.util.WriteFileByOffSet(fpath, fileInfo.OffSet, data)
		if err != nil {
			log.Warn(err)
		}
		this.SaveFileMd5Log(fileInfo, CONST_FILE_Md5_FILE_NAME)
		return
	}
	if err = req.ToFile(fpath); err != nil {
		log.Error(err)
		return
	}
	if fi, err = os.Stat(fpath); err != nil {
		os.Remove(fpath)
		return
	}
	if sum, err = this.util.GetFileSumByName(fpath, Config().FileSumArithmetic); err != nil {
		log.Error(err)
		return
	}
	if fi.Size() != fileInfo.Size || sum != fileInfo.Md5 {
		log.Error("file sum check error")
		os.Remove(fpath)
		return
	}
	if this.util.IsExist(fpath) {
		this.SaveFileMd5Log(fileInfo, CONST_FILE_Md5_FILE_NAME)
	}
}
func (this *Server) Download(w http.ResponseWriter, r *http.Request) {
	var (
		err          error
		pathMd5      string
		info         os.FileInfo
		peer         string
		fileInfo     *FileInfo
		fullpath     string
		pathval      url.Values
		token        string
		timestamp    string
		maxTimestamp int64
		minTimestamp int64
		ts           int64
		md5sum       string
		fp           *os.File
		isPeer       bool
		isSmallFile  bool
		data         []byte
		offset       int64
		length       int
		smallPath    string
		notFound     bool
		//isBigFile    bool
	)
	r.ParseForm()
	isPeer = this.IsPeer(r)
	if Config().DownloadUseToken && !isPeer {
		token = r.FormValue("token")
		timestamp = r.FormValue("timestamp")
		if token == "" || timestamp == "" {
			w.Write([]byte("unvalid request"))
			return
		}
		maxTimestamp = time.Now().Add(time.Second *
			time.Duration(Config().DownloadTokenExpire)).Unix()
		minTimestamp = time.Now().Add(-time.Second *
			time.Duration(Config().DownloadTokenExpire)).Unix()
		if ts, err = strconv.ParseInt(timestamp, 10, 64); err != nil {
			w.Write([]byte("unvalid timestamp"))
			return
		}
		if ts > maxTimestamp || ts < minTimestamp {
			w.Write([]byte("timestamp expire"))
			return
		}
	}
	fullpath = r.RequestURI[len(Config().Group)+2 : len(r.RequestURI)]
	fullpath = DOCKER_DIR + STORE_DIR_NAME + "/" + fullpath
	//fmt.Println("fullpath",fullpath)
	if strings.HasPrefix(r.RequestURI, "/"+Config().Group+"/"+LARGE_DIR_NAME+"/") {
		isSmallFile = true
		smallPath = fullpath //notice order
		fullpath = strings.Split(fullpath, ",")[0]
	}
	_ = isSmallFile
	_ = smallPath
	fullpath = strings.Replace(fullpath, "&", "$$$$", -1)
	if pathval, err = url.ParseQuery(fullpath); err != nil {
		log.Error(err)
	} else {
		for k, v := range pathval {
			if k != "" {
				if len(v) > 0 && v[0] != "" {
					fullpath = k + "=" + v[0]
				} else {
					fullpath = k
				}
			}
		}
	}
	fullpath = strings.Replace(fullpath, "$$$$", "&", -1)
	CheckToken := func(token string, md5sum string, timestamp string) bool {
		if this.util.MD5(md5sum+timestamp) != token {
			return false
		}
		return true
	}
	if Config().DownloadUseToken && !isPeer {
		if isSmallFile {
			pathMd5 = this.util.MD5(smallPath)
		} else {
			fullpath = strings.Split(fullpath, "?")[0]
			pathMd5 = this.util.MD5(fullpath)
		}
		if fileInfo, err = this.GetFileInfoFromLevelDB(pathMd5); err != nil {
			log.Error(err)
			if this.util.FileExists(fullpath) {
				if fp, err = os.Create(fullpath); err != nil {
					log.Error(err)
				}
				if fp != nil {
					defer fp.Close()
				}
				md5sum = this.util.GetFileSum(fp, Config().FileSumArithmetic)
				if !CheckToken(token, md5sum, timestamp) {
					w.Write([]byte("unvalid request,error token"))
					return
				}
			}
		} else {
			if !CheckToken(token, fileInfo.Md5, timestamp) {
				w.Write([]byte("unvalid request,error token"))
				return
			}
		}
	}
	if isSmallFile {
		if _, offset, length, err = this.ParseSmallFile(r.RequestURI); err != nil {
			log.Error(err)
			return
		}
		if info, err = os.Stat(fullpath); err != nil {
			notFound = true
			goto NotFound // if return can't not repair file
			return
		}
		if info.Size() < offset+int64(length) {
			notFound = true
		} else {
			data, err = this.util.ReadFileByOffSet(fullpath, offset, length)
			if err != nil {
				log.Error(err)
				return
			}
			if string(data[0]) == "1" {
				w.Write(data[1:])
				return
			} else {
				notFound = true
			}
		}
	}
NotFound:
	if info, err = os.Stat(fullpath); err != nil || info.Size() == 0 || notFound {
		log.Error(err)
		if isSmallFile && notFound {
			pathMd5 = this.util.MD5(smallPath)
		} else {
			pathMd5 = this.util.MD5(fullpath)
		}
		for _, peer = range Config().Peers {
			if fileInfo, err = this.checkPeerFileExist(peer, pathMd5); err != nil {
				log.Error(err)
				continue
			}
			if fileInfo.Md5 != "" {
				if Config().DownloadUseToken && !isPeer {
					if !CheckToken(token, fileInfo.Md5, timestamp) {
						w.Write([]byte("unvalid request,error token"))
						return
					}
				}
				go this.DownloadFromPeer(peer, fileInfo)
				http.Redirect(w, r, peer+r.RequestURI, 302)
				return
			}
		}
		w.WriteHeader(404)
		return
	}
	if !Config().ShowDir && info.IsDir() {
		w.Write([]byte("list dir deny"))
		return
	}
	log.Info("download:" + r.RequestURI)
	staticHandler.ServeHTTP(w, r)
}
func (this *Server) GetServerURI(r *http.Request) string {
	return fmt.Sprintf("http://%s/", r.Host)
}
func (this *Server) CheckFileAndSendToPeer(date string, filename string, isForceUpload bool) {
	var (
		md5set mapset.Set
		err    error
		md5s   []interface{}
	)
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("CheckFileAndSendToPeer")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	if md5set, err = this.GetMd5sByDate(date, filename); err != nil {
		log.Error(err)
		return
	}
	md5s = md5set.ToSlice()
	for _, md := range md5s {
		if md == nil {
			continue
		}
		if fileInfo, _ := this.GetFileInfoFromLevelDB(md.(string)); fileInfo != nil && fileInfo.Md5 != "" {
			if isForceUpload {
				fileInfo.Peers = []string{}
			}
			if len(fileInfo.Peers) > len(Config().Peers) {
				continue
			}
			if !this.util.Contains(this.host, fileInfo.Peers) {
				fileInfo.Peers = append(fileInfo.Peers, this.host) // peer is null
			}
			if filename == CONST_Md5_QUEUE_FILE_NAME {
				this.AppendToDownloadQueue(fileInfo)
			} else {
				this.AppendToQueue(fileInfo)
			}
		}
	}
}
func (this *Server) postFileToPeer(fileInfo *FileInfo) {
	var (
		err      error
		peer     string
		filename string
		info     *FileInfo
		postURL  string
		result   string
		fi       os.FileInfo
		i        int
		data     []byte
		fpath    string
	)
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("postFileToPeer")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	//fmt.Println("postFile",fileInfo)
	for i, peer = range Config().Peers {
		_ = i
		if fileInfo.Peers == nil {
			fileInfo.Peers = []string{}
		}
		if this.util.Contains(peer, fileInfo.Peers) {
			continue
		}
		filename = fileInfo.Name
		if fileInfo.ReName != "" {
			filename = fileInfo.ReName
			if fileInfo.OffSet != -1 {
				filename = strings.Split(fileInfo.ReName, ",")[0]
			}
		}
		fpath = DOCKER_DIR + fileInfo.Path + "/" + filename
		if !this.util.FileExists(fpath) {
			log.Warn(fmt.Sprintf("file '%s' not found", fpath))
			continue
		} else {
			if fileInfo.Size == 0 {
				if fi, err = os.Stat(fpath); err != nil {
					log.Error(err)
				} else {
					fileInfo.Size = fi.Size()
				}
			}
		}
		if info, err = this.checkPeerFileExist(peer, fileInfo.Md5); info.Md5 != "" {
			fileInfo.Peers = append(fileInfo.Peers, peer)
			if _, err = this.SaveFileInfoToLevelDB(fileInfo.Md5, fileInfo, this.ldb); err != nil {
				log.Error(err)
			}
			continue
		}
		postURL = fmt.Sprintf("%s%s", peer, this.getRequestURI("syncfile_info"))
		b := httplib.Post(postURL)
		b.SetTimeout(time.Second*5, time.Second*5)
		if data, err = json.Marshal(fileInfo); err != nil {
			log.Error(err)
			return
		}
		b.Param("fileInfo", string(data))
		result, err = b.String()
		if !strings.HasPrefix(result, "http://") || err != nil {
			this.SaveFileMd5Log(fileInfo, CONST_Md5_ERROR_FILE_NAME)
		}
		if strings.HasPrefix(result, "http://") {
			log.Info(result)
			if !this.util.Contains(peer, fileInfo.Peers) {
				fileInfo.Peers = append(fileInfo.Peers, peer)
				if _, err = this.SaveFileInfoToLevelDB(fileInfo.Md5, fileInfo, this.ldb); err != nil {
					log.Error(err)
				}
			}
		}
		if err != nil {
			log.Error(err)
		}
	}
}
func (this *Server) SaveFileMd5Log(fileInfo *FileInfo, filename string) {
	var (
		info FileInfo
	)
	info = *fileInfo
	this.queueFileLog <- &FileLog{FileInfo: &info, FileName: filename}
}
func (this *Server) saveFileMd5Log(fileInfo *FileInfo, filename string) {
	var (
		err      error
		outname  string
		logDate  string
		ok       bool
		fullpath string
		md5Path  string
		logKey   string
	)
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("saveFileMd5Log")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	if fileInfo == nil || fileInfo.Md5 == "" || filename == "" {
		log.Warn("saveFileMd5Log", fileInfo, filename)
		return
	}
	logDate = this.util.GetDayFromTimeStamp(fileInfo.TimeStamp)
	outname = fileInfo.Name
	if fileInfo.ReName != "" {
		outname = fileInfo.ReName
	}
	fullpath = fileInfo.Path + "/" + outname
	logKey = fmt.Sprintf("%s_%s_%s", logDate, filename, fileInfo.Md5)
	if filename == CONST_FILE_Md5_FILE_NAME {
		if ok, err = this.IsExistFromLevelDB(fileInfo.Md5, this.ldb); !ok {
			this.statMap.AddCountInt64(logDate+"_"+CONST_STAT_FILE_COUNT_KEY, 1)
			this.statMap.AddCountInt64(logDate+"_"+CONST_STAT_FILE_TOTAL_SIZE_KEY, fileInfo.Size)
			this.SaveStat()
		}
		if _, err = this.SaveFileInfoToLevelDB(logKey, fileInfo, this.logDB); err != nil {
			log.Error(err)
		}
		if _, err := this.SaveFileInfoToLevelDB(fileInfo.Md5, fileInfo, this.ldb); err != nil {
			log.Error("saveToLevelDB", err, fileInfo)
		}
		if _, err = this.SaveFileInfoToLevelDB(this.util.MD5(fullpath), fileInfo, this.ldb); err != nil {
			log.Error("saveToLevelDB", err, fileInfo)
		}
		return
	}
	if filename == CONST_REMOME_Md5_FILE_NAME {
		if ok, err = this.IsExistFromLevelDB(fileInfo.Md5, this.ldb); ok {
			this.statMap.AddCountInt64(logDate+"_"+CONST_STAT_FILE_COUNT_KEY, -1)
			this.statMap.AddCountInt64(logDate+"_"+CONST_STAT_FILE_TOTAL_SIZE_KEY, -fileInfo.Size)
			this.SaveStat()
		}
		this.RemoveKeyFromLevelDB(logKey, this.logDB)
		md5Path = this.util.MD5(fullpath)
		if err := this.RemoveKeyFromLevelDB(fileInfo.Md5, this.ldb); err != nil {
			log.Error("RemoveKeyFromLevelDB", err, fileInfo)
		}
		if err = this.RemoveKeyFromLevelDB(md5Path, this.ldb); err != nil {
			log.Error("RemoveKeyFromLevelDB", err, fileInfo)
		}
		return
	}
	this.SaveFileInfoToLevelDB(logKey, fileInfo, this.logDB)
	if filename == CONST_Md5_QUEUE_FILE_NAME {
		this.queueFromPeers <- *fileInfo
		return
	}
}
func (this *Server) checkPeerFileExist(peer string, md5sum string) (*FileInfo, error) {
	var (
		err      error
		fileInfo FileInfo
	)
	req := httplib.Post(fmt.Sprintf("%s%s?md5=%s", peer, this.getRequestURI("check_file_exist"), md5sum))
	req.SetTimeout(time.Second*5, time.Second*10)
	if err = req.ToJSON(&fileInfo); err != nil {
		return &FileInfo{}, err
	}
	if fileInfo.Md5 == "" {
		return &fileInfo, errors.New("not found")
	}
	return &fileInfo, nil
}
func (this *Server) CheckFileExist(w http.ResponseWriter, r *http.Request) {
	var (
		data     []byte
		err      error
		fileInfo *FileInfo
		fpath    string
	)
	r.ParseForm()
	md5sum := ""
	md5sum = r.FormValue("md5")
	if fileInfo, err = this.GetFileInfoFromLevelDB(md5sum); fileInfo != nil {
		if fileInfo.OffSet != -1 {
			if data, err = json.Marshal(fileInfo); err != nil {
				log.Error(err)
			}
			w.Write(data)
			return
		}
		fpath = DOCKER_DIR + fileInfo.Path + "/" + fileInfo.Name
		if fileInfo.ReName != "" {
			fpath = DOCKER_DIR + fileInfo.Path + "/" + fileInfo.ReName
		}
		if this.util.IsExist(fpath) {
			if data, err = json.Marshal(fileInfo); err == nil {
				w.Write(data)
				return
			} else {
				log.Error(err)
			}
		} else {
			if fileInfo.OffSet == -1 {
				this.RemoveKeyFromLevelDB(md5sum, this.ldb) // when file delete,delete from leveldb
			}
		}
	}
	data, _ = json.Marshal(FileInfo{})
	w.Write(data)
	return
}
func (this *Server) Sync(w http.ResponseWriter, r *http.Request) {
	var (
		result JsonResult
	)
	r.ParseForm()
	result.Status = "fail"
	if !this.IsPeer(r) {
		result.Message = "client must be in cluster"
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	date := ""
	force := ""
	inner := ""
	isForceUpload := false
	force = r.FormValue("force")
	date = r.FormValue("date")
	inner = r.FormValue("inner")
	if force == "1" {
		isForceUpload = true
	}
	if inner != "1" {
		for _, peer := range Config().Peers {
			req := httplib.Post(peer + this.getRequestURI("sync"))
			req.Param("force", force)
			req.Param("inner", "1")
			req.Param("date", date)
			if _, err := req.String(); err != nil {
				log.Error(err)
			}
		}
	}
	if date == "" {
		result.Message = "require paramete date &force , ?date=20181230"
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	date = strings.Replace(date, ".", "", -1)
	if isForceUpload {
		go this.CheckFileAndSendToPeer(date, CONST_FILE_Md5_FILE_NAME, isForceUpload)
	} else {
		go this.CheckFileAndSendToPeer(date, CONST_Md5_ERROR_FILE_NAME, isForceUpload)
	}
	result.Status = "ok"
	result.Message = "job is running"
	w.Write([]byte(this.util.JsonEncodePretty(result)))
}
func (this *Server) IsExistFromLevelDB(key string, db *leveldb.DB) (bool, error) {
	return db.Has([]byte(key), nil)
}
func (this *Server) GetFileInfoFromLevelDB(key string) (*FileInfo, error) {
	var (
		err      error
		data     []byte
		fileInfo FileInfo
	)
	if data, err = this.ldb.Get([]byte(key), nil); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &fileInfo); err != nil {
		return nil, err
	}
	return &fileInfo, nil
}
func (this *Server) SaveStat() {
	SaveStatFunc := func() {
		defer func() {
			if re := recover(); re != nil {
				buffer := debug.Stack()
				log.Error("SaveStatFunc")
				log.Error(re)
				log.Error(string(buffer))
			}
		}()
		stat := this.statMap.Get()
		if v, ok := stat[CONST_STAT_FILE_COUNT_KEY]; ok {
			switch v.(type) {
			case int64, int32, int, float64, float32:
				if v.(int64) >= 0 {
					if data, err := json.Marshal(stat); err != nil {
						log.Error(err)
					} else {
						this.util.WriteBinFile(CONST_STAT_FILE_NAME, data)
					}
				}
			}
		}
	}
	SaveStatFunc()
}
func (this *Server) RemoveKeyFromLevelDB(key string, db *leveldb.DB) (error) {
	var (
		err error
	)
	err = db.Delete([]byte(key), nil)
	return err
}
func (this *Server) SaveFileInfoToLevelDB(key string, fileInfo *FileInfo, db *leveldb.DB) (*FileInfo, error) {
	var (
		err  error
		data []byte
	)
	if fileInfo == nil || db == nil {
		return nil, errors.New("fileInfo is null or db is null")
	}
	if data, err = json.Marshal(fileInfo); err != nil {
		return fileInfo, err
	}
	if err = db.Put([]byte(key), data, nil); err != nil {
		return fileInfo, err
	}
	return fileInfo, nil
}
func (this *Server) IsPeer(r *http.Request) bool {
	var (
		ip    string
		peer  string
		bflag bool
	)
	//return true
	ip = this.util.GetClientIp(r)
	if ip == "127.0.0.1" || ip == this.util.GetPulicIP() {
		return true
	}
	if this.util.Contains(ip, Config().AdminIps) {
		return true
	}
	ip = "http://" + ip
	bflag = false
	for _, peer = range Config().Peers {
		if strings.HasPrefix(peer, ip) {
			bflag = true
			break
		}
	}
	return bflag
}
func (this *Server) ReceiveMd5s(w http.ResponseWriter, r *http.Request) {
	var (
		err      error
		md5str   string
		fileInfo *FileInfo
		md5s     []string
	)
	if !this.IsPeer(r) {
		log.Warn(fmt.Sprintf("ReceiveMd5s %s", this.util.GetClientIp(r)))
		w.Write([]byte(this.GetClusterNotPermitMessage(r)))
		return
	}
	r.ParseForm()
	md5str = r.FormValue("md5s")
	md5s = strings.Split(md5str, ",")
	AppendFunc := func(md5s []string) {
		for _, m := range md5s {
			if m != "" {
				if fileInfo, err = this.GetFileInfoFromLevelDB(m); err != nil {
					log.Error(err)
					continue
				}
				this.AppendToQueue(fileInfo)
			}
		}
	}
	go AppendFunc(md5s)
}
func (this *Server) GetClusterNotPermitMessage(r *http.Request) string {
	var (
		message string
	)
	message = fmt.Sprintf(CONST_MESSAGE_CLUSTER_IP, this.util.GetClientIp(r))
	return message
}
func (this *Server) GetMd5sForWeb(w http.ResponseWriter, r *http.Request) {
	var (
		date   string
		err    error
		result mapset.Set
		lines  []string
		md5s   []interface{}
	)
	if !this.IsPeer(r) {
		w.Write([]byte(this.GetClusterNotPermitMessage(r)))
		return
	}
	date = r.FormValue("date")
	if result, err = this.GetMd5sByDate(date, CONST_FILE_Md5_FILE_NAME); err != nil {
		log.Error(err)
		return
	}
	md5s = result.ToSlice()
	for _, line := range md5s {
		if line != nil && line != "" {
			lines = append(lines, line.(string))
		}
	}
	w.Write([]byte( strings.Join(lines, ",") ))
}
func (this *Server) GetMd5File(w http.ResponseWriter, r *http.Request) {
	var (
		date  string
		fpath string
		data  []byte
		err   error
	)
	if !this.IsPeer(r) {
		return
	}
	fpath = DATA_DIR + "/" + date + "/" + CONST_FILE_Md5_FILE_NAME
	if !this.util.FileExists(fpath) {
		w.WriteHeader(404)
		return
	}
	if data, err = ioutil.ReadFile(fpath); err != nil {
		w.WriteHeader(500)
		return
	}
	w.Write(data)
}
func (this *Server) GetMd5sMapByDate(date string, filename string) (*CommonMap, error) {
	var (
		err     error
		result  *CommonMap
		fpath   string
		content string
		lines   []string
		line    string
		cols    []string
		data    []byte
	)
	result = &CommonMap{m: make(map[string]interface{})}
	if filename == "" {
		fpath = DATA_DIR + "/" + date + "/" + CONST_FILE_Md5_FILE_NAME
	} else {
		fpath = DATA_DIR + "/" + date + "/" + filename
	}
	if !this.util.FileExists(fpath) {
		return result, errors.New(fmt.Sprintf("fpath %s not found", fpath))
	}
	if data, err = ioutil.ReadFile(fpath); err != nil {
		return result, err
	}
	content = string(data)
	lines = strings.Split(content, "\n")
	for _, line = range lines {
		cols = strings.Split(line, "|")
		if len(cols) > 2 {
			if _, err = strconv.ParseInt(cols[1], 10, 64); err != nil {
				continue
			}
			result.Add(cols[0])
		}
	}
	return result, nil
}
func (this *Server) GetMd5sByDate(date string, filename string) (mapset.Set, error) {
	var (
		keyPrefix string
		md5set    mapset.Set
		keys      []string
	)
	md5set = mapset.NewSet()
	keyPrefix = "%s_%s_"
	keyPrefix = fmt.Sprintf(keyPrefix, date, filename)
	iter := server.logDB.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
	for iter.Next() {
		keys = strings.Split(string(iter.Key()), "_")
		if len(keys) >= 3 {
			md5set.Add(keys[2])
		}
	}
	iter.Release()
	return md5set, nil
}
func (this *Server) SyncFileInfo(w http.ResponseWriter, r *http.Request) {
	var (
		err         error
		fileInfo    FileInfo
		fileInfoStr string
		filename    string
	)
	r.ParseForm()
	if !this.IsPeer(r) {
		return
	}
	fileInfoStr = r.FormValue("fileInfo")
	if err = json.Unmarshal([]byte(fileInfoStr), &fileInfo); err != nil {
		w.Write([]byte(this.GetClusterNotPermitMessage(r)))
		log.Error(err)
		return
	}
	this.SaveFileMd5Log(&fileInfo, CONST_Md5_QUEUE_FILE_NAME)
	go this.AppendToDownloadQueue(&fileInfo)
	filename = fileInfo.Name
	if fileInfo.ReName != "" {
		filename = fileInfo.ReName
	}
	p := strings.Replace(fileInfo.Path, STORE_DIR+"/", "", 1)
	downloadUrl := fmt.Sprintf("http://%s/%s", r.Host, Config().Group+"/"+p+"/"+filename)
	w.Write([]byte(downloadUrl))
}
func (this *Server) CheckScene(scene string) (bool, error) {
	if len(Config().Scenes) == 0 {
		return true, nil
	}
	if !this.util.Contains(scene, Config().Scenes) {
		return false, errors.New("not valid scene")
	}
	return true, nil
}
func (this *Server) RemoveFile(w http.ResponseWriter, r *http.Request) {
	var (
		err      error
		md5sum   string
		fileInfo *FileInfo
		fpath    string
		delUrl   string
		result   JsonResult
		inner    string
		name     string
	)
	_ = delUrl
	_ = inner
	r.ParseForm()
	md5sum = r.FormValue("md5")
	fpath = r.FormValue("path")
	inner = r.FormValue("inner")
	result.Status = "fail"
	if fpath != "" && md5sum == "" {
		fpath = strings.Replace(fpath, "/"+Config().Group+"/", STORE_DIR_NAME+"/", 1)
		md5sum = this.util.MD5(fpath)
	}
	if inner != "1" {
		for _, peer := range Config().Peers {
			delFile := func(peer string, md5sum string, fileInfo *FileInfo) {
				delUrl = fmt.Sprintf("%s%s", peer, this.getRequestURI("delete"))
				req := httplib.Post(delUrl)
				req.Param("md5", md5sum)
				req.Param("inner", "1")
				req.SetTimeout(time.Second*5, time.Second*10)
				if _, err = req.String(); err != nil {
					log.Error(err)
				}
			}
			go delFile(peer, md5sum, fileInfo)
		}
	}
	if len(md5sum) < 32 {
		result.Message = "md5 unvalid"
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	if fileInfo, err = this.GetFileInfoFromLevelDB(md5sum); err != nil {
		result.Message = err.Error()
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	if fileInfo.OffSet != -1 {
		result.Message = "small file delete not support"
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	name = fileInfo.Name
	if fileInfo.ReName != "" {
		name = fileInfo.ReName
	}
	fpath = fileInfo.Path + "/" + name
	if fileInfo.Path != "" && this.util.FileExists(DOCKER_DIR+fpath) {
		this.SaveFileMd5Log(fileInfo, CONST_REMOME_Md5_FILE_NAME)
		if err = os.Remove(DOCKER_DIR + fpath); err != nil {
			result.Message = err.Error()
			w.Write([]byte(this.util.JsonEncodePretty(result)))
			return
		} else {
			result.Message = "remove success"
			result.Status = "ok"
			w.Write([]byte(this.util.JsonEncodePretty(result)))
			return
		}
	}
	result.Message = "fail remove"
	w.Write([]byte(this.util.JsonEncodePretty(result)))
}
func (this *Server) getRequestURI(action string) string {
	var (
		uri string
	)
	if Config().SupportGroupManage {
		uri = "/" + Config().Group + "/" + action
	} else {
		uri = "/" + action
	}
	return uri
}
func (this *Server) BuildFileResult(fileInfo *FileInfo, r *http.Request) FileResult {
	var (
		outname     string
		fileResult  FileResult
		p           string
		downloadUrl string
		domain      string
	)
	if Config().DownloadDomain != "" {
		domain = fmt.Sprintf("http://%s", Config().DownloadDomain)
	} else {
		domain = fmt.Sprintf("http://%s", r.Host)
	}
	outname = fileInfo.Name
	if fileInfo.ReName != "" {
		outname = fileInfo.ReName
	}
	p = strings.Replace(fileInfo.Path, STORE_DIR_NAME+"/", "", 1)
	p = Config().Group + "/" + p + "/" + outname
	downloadUrl = fmt.Sprintf("http://%s/%s", r.Host, p)
	if Config().DownloadDomain != "" {
		downloadUrl = fmt.Sprintf("http://%s/%s", Config().DownloadDomain, p)
	}
	fileResult.Url = downloadUrl
	fileResult.Md5 = fileInfo.Md5
	fileResult.Path = "/" + p
	fileResult.Domain = domain
	fileResult.Scene = fileInfo.Scene
	// Just for Compatibility
	fileResult.Src = fileResult.Path
	fileResult.Scenes = fileInfo.Scene
	return fileResult
}
func (this *Server) SaveUploadFile(file multipart.File, header *multipart.FileHeader, fileInfo *FileInfo, r *http.Request) (*FileInfo, error) {
	var (
		err     error
		outFile *os.File
		folder  string
		fi      os.FileInfo
	)
	defer file.Close()
	fileInfo.Name = header.Filename
	if Config().RenameFile {
		fileInfo.ReName = this.util.MD5(this.util.GetUUID()) + path.Ext(fileInfo.Name)
	}
	folder = time.Now().Format("20060102/15/04")
	if Config().PeerId != "" {
		folder = fmt.Sprintf(folder+"/%s", Config().PeerId)
	}
	if fileInfo.Scene != "" {
		folder = fmt.Sprintf(STORE_DIR+"/%s/%s", fileInfo.Scene, folder)
	} else {
		folder = fmt.Sprintf(STORE_DIR+"/%s", folder)
	}
	if fileInfo.Path != "" {
		if strings.HasPrefix(fileInfo.Path, STORE_DIR) {
			folder = fileInfo.Path
		} else {
			folder = STORE_DIR + "/" + fileInfo.Path
		}
	}
	if !this.util.FileExists(folder) {
		os.MkdirAll(folder, 0775)
	}
	outPath := fmt.Sprintf(folder+"/%s", fileInfo.Name)
	if Config().RenameFile {
		outPath = fmt.Sprintf(folder+"/%s", fileInfo.ReName)
	}
	if this.util.FileExists(outPath) {
		for i := 0; i < 10000; i++ {
			outPath = fmt.Sprintf(folder+"/%d_%s", i, header.Filename)
			fileInfo.Name = fmt.Sprintf("%d_%s", i, header.Filename)
			if !this.util.FileExists(outPath) {
				break
			}
		}
	}
	log.Info(fmt.Sprintf("upload: %s", outPath))
	if outFile, err = os.Create(outPath); err != nil {
		return fileInfo, err
	}
	defer outFile.Close()
	if err != nil {
		log.Error(err)
		return fileInfo, errors.New("(error)fail," + err.Error())
	}
	if _, err = io.Copy(outFile, file); err != nil {
		log.Error(err)
		return fileInfo, errors.New("(error)fail," + err.Error())
	}
	if fi, err = outFile.Stat(); err != nil {
		log.Error(err)
	} else {
		fileInfo.Size = fi.Size()
	}
	if fi.Size() != header.Size {
		return fileInfo, errors.New("(error)file uncomplete")
	}
	v := this.util.GetFileSum(outFile, Config().FileSumArithmetic)
	fileInfo.Md5 = v
	//fileInfo.Path = folder //strings.Replace( folder,DOCKER_DIR,"",1)
	fileInfo.Path = strings.Replace(folder, DOCKER_DIR, "", 1)
	fileInfo.Peers = append(fileInfo.Peers, fmt.Sprintf("http://%s", r.Host))
	//fmt.Println("upload",fileInfo)
	return fileInfo, nil
}
func (this *Server) Upload(w http.ResponseWriter, r *http.Request) {
	var (
		err error
		//		pathname     string
		md5sum       string
		fileInfo     FileInfo
		uploadFile   multipart.File
		uploadHeader *multipart.FileHeader
		scene        string
		output       string
		fileResult   FileResult
		data         []byte
	)
	//r.ParseForm()
	if r.Method == "POST" {
		//		name := r.PostFormValue("name")
		//		fileInfo.Path = r.Header.Get("Sync-Path")
		md5sum = r.FormValue("md5")
		output = r.FormValue("output")
		//if strings.Contains(r.Host, "127.0.0.1") {
		//	w.Write([]byte( "(error) upload use clust ip(peers ip),not 127.0.0.1"))
		//	return
		//}
		if Config().EnableCustomPath {
			fileInfo.Path = r.FormValue("path")
			fileInfo.Path = strings.Trim(fileInfo.Path, "/")
		}
		scene = r.FormValue("scene")
		if scene == "" {
			//Just for Compatibility
			scene = r.FormValue("scenes")
		}
		fileInfo.Md5 = md5sum
		fileInfo.OffSet = -1
		if uploadFile, uploadHeader, err = r.FormFile("file"); err != nil {
			log.Error(err)
			w.Write([]byte(err.Error()))
			return
		}
		fileInfo.Peers = []string{}
		fileInfo.TimeStamp = time.Now().Unix()
		if scene == "" {
			scene = Config().DefaultScene
		}
		if output == "" {
			output = "text"
		}
		if !this.util.Contains(output, []string{"json", "text"}) {
			w.Write([]byte("output just support json or text"))
			return
		}
		fileInfo.Scene = scene
		if _, err = this.CheckScene(scene); err != nil {
			w.Write([]byte(err.Error()))
			return
		}
		if err != nil {
			log.Error(err)
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
			return
		}
		if _, err = this.SaveUploadFile(uploadFile, uploadHeader, &fileInfo, r); err != nil {
			w.Write([]byte(err.Error()))
			return
		}
		if v, _ := this.GetFileInfoFromLevelDB(fileInfo.Md5); v != nil && v.Md5 != "" {
			fileResult = this.BuildFileResult(v, r)
			if Config().RenameFile {
				os.Remove(DOCKER_DIR + fileInfo.Path + "/" + fileInfo.ReName)
			} else {
				os.Remove(DOCKER_DIR + fileInfo.Path + "/" + fileInfo.Name)
			}
			if output == "json" {
				if data, err = json.Marshal(fileResult); err != nil {
					log.Error(err)
					w.Write([]byte(err.Error()))
				}
				w.Write(data)
			} else {
				w.Write([]byte(fileResult.Url))
			}
			return
		}
		if fileInfo.Md5 == "" {
			log.Warn(" fileInfo.Md5 is null")
			return
		}
		if md5sum != "" && fileInfo.Md5 != md5sum {
			log.Warn(" fileInfo.Md5 and md5sum !=")
			return
		}
		if Config().EnableMergeSmallFile && fileInfo.Size < CONST_SMALL_FILE_SIZE {
			if err = this.SaveSmallFile(&fileInfo); err != nil {
				log.Error(err)
				return
			}
		}
		go this.postFileToPeer(&fileInfo)
		if fileInfo.Size <= 0 {
			log.Error("file size is zero")
			return
		}
		fileResult = this.BuildFileResult(&fileInfo, r)
		this.SaveFileMd5Log(&fileInfo, CONST_FILE_Md5_FILE_NAME)
		if output == "json" {
			if data, err = json.Marshal(fileResult); err != nil {
				log.Error(err)
				w.Write([]byte(err.Error()))
			}
			w.Write(data)
		} else {
			w.Write([]byte(fileResult.Url))
		}
		return
	} else {
		md5sum = r.FormValue("md5")
		output = r.FormValue("output")
		if md5sum == "" {
			w.Write([]byte("(error) if you want to upload fast md5 is require" +
				",and if you want to upload file,you must use post method  "))
			return
		}
		if v, _ := this.GetFileInfoFromLevelDB(md5sum); v != nil && v.Md5 != "" {
			fileResult = this.BuildFileResult(v, r)
			if output == "json" {
				if data, err = json.Marshal(fileResult); err != nil {
					log.Error(err)
					w.Write([]byte(err.Error()))
				}
				w.Write(data)
			} else {
				w.Write([]byte(fileResult.Url))
			}
			return
		}
		w.Write([]byte("(error)fail,please use post method"))
		return
	}
}
func (this *Server) SaveSmallFile(fileInfo *FileInfo) (error) {
	var (
		err      error
		filename string
		fpath    string
		srcFile  *os.File
		desFile  *os.File
		largeDir string
		destPath string
		reName   string
		fileExt  string
	)
	filename = fileInfo.Name
	fileExt = path.Ext(filename)
	if fileInfo.ReName != "" {
		filename = fileInfo.ReName
	}
	fpath = DOCKER_DIR + fileInfo.Path + "/" + filename
	largeDir = LARGE_DIR + "/" + Config().PeerId
	if !this.util.FileExists(largeDir) {
		os.MkdirAll(largeDir, 0775)
	}
	reName = fmt.Sprintf("%d", this.util.RandInt(100, 300))
	destPath = largeDir + "/" + reName
	this.lockMap.LockKey(destPath)
	defer this.lockMap.UnLockKey(destPath)
	if this.util.FileExists(fpath) {
		srcFile, err = os.OpenFile(fpath, os.O_CREATE|os.O_RDONLY, 06666)
		if err != nil {
			return err
		}
		defer srcFile.Close()
		desFile, err = os.OpenFile(destPath, os.O_CREATE|os.O_RDWR, 06666)
		if err != nil {
			return err
		}
		defer desFile.Close()
		fileInfo.OffSet, err = desFile.Seek(0, 2)
		if _, err = desFile.Write([]byte("1")); err != nil { //first byte set 1
			return err
		}
		fileInfo.OffSet, err = desFile.Seek(0, 2)
		if err != nil {
			return err
		}
		fileInfo.OffSet = fileInfo.OffSet - 1 //minus 1 byte
		fileInfo.Size = fileInfo.Size + 1
		fileInfo.ReName = fmt.Sprintf("%s,%d,%d,%s", reName, fileInfo.OffSet, fileInfo.Size, fileExt)
		if _, err = io.Copy(desFile, srcFile); err != nil {
			return err
		}
		srcFile.Close()
		os.Remove(fpath)
		fileInfo.Path = strings.Replace(largeDir, DOCKER_DIR, "", 1)
	}
	return nil
}
func (this *Server) SendToMail(to, subject, body, mailtype string) error {
	host := Config().Mail.Host
	user := Config().Mail.User
	password := Config().Mail.Password
	hp := strings.Split(host, ":")
	auth := smtp.PlainAuth("", user, password, hp[0])
	var contentType string
	if mailtype == "html" {
		contentType = "Content-Type: text/" + mailtype + "; charset=UTF-8"
	} else {
		contentType = "Content-Type: text/plain" + "; charset=UTF-8"
	}
	msg := []byte("To: " + to + "\r\nFrom: " + user + ">\r\nSubject: " + "\r\n" + contentType + "\r\n\r\n" + body)
	sendTo := strings.Split(to, ";")
	err := smtp.SendMail(host, auth, user, sendTo, msg)
	return err
}
func (this *Server) BenchMark(w http.ResponseWriter, r *http.Request) {
	t := time.Now()
	batch := new(leveldb.Batch)
	for i := 0; i < 100000000; i++ {
		f := FileInfo{}
		f.Peers = []string{"http://192.168.0.1", "http://192.168.2.5"}
		f.Path = "20190201/19/02"
		s := strconv.Itoa(i)
		s = this.util.MD5(s)
		f.Name = s
		f.Md5 = s
		if data, err := json.Marshal(&f); err == nil {
			batch.Put([]byte(s), data)
		}
		if i%10000 == 0 {
			if batch.Len() > 0 {
				server.ldb.Write(batch, nil)
				//				batch = new(leveldb.Batch)
				batch.Reset()
			}
			fmt.Println(i, time.Since(t).Seconds())
		}
		//fmt.Println(server.GetFileInfoFromLevelDB(s))
	}
	this.util.WriteFile("time.txt", time.Since(t).String())
	fmt.Println(time.Since(t).String())
}
func (this *Server) RepairStatWeb(w http.ResponseWriter, r *http.Request) {
	var (
		result JsonResult
		date   string
		inner  string
	)
	if !this.IsPeer(r) {
		result.Message = this.GetClusterNotPermitMessage(r)
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	date = r.FormValue("date")
	inner = r.FormValue("inner")
	if date == "" {
		date = this.util.GetToDay()
	}
	if inner != "1" {
		for _, peer := range Config().Peers {
			req := httplib.Post(peer + this.getRequestURI("repair_stat"))
			req.Param("inner", "1")
			req.Param("date", date)
			if _, err := req.String(); err != nil {
				log.Error(err)
			}
		}
	}
	result.Data = this.RepairStatByDate(date)
	result.Status = "ok"
	w.Write([]byte(this.util.JsonEncodePretty(result)))
}
func (this *Server) Stat(w http.ResponseWriter, r *http.Request) {
	var (
		result JsonResult
		inner  string
	)
	if !this.IsPeer(r) {
		result.Message = this.GetClusterNotPermitMessage(r)
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	r.ParseForm()
	inner = r.FormValue("inner")
	data := this.GetStat()
	result.Status = "ok"
	result.Data = data
	if inner == "1" {
		w.Write([]byte(this.util.JsonEncodePretty(data)))
	} else {
		w.Write([]byte(this.util.JsonEncodePretty(result)))
	}
}
func (this *Server) GetStat() []StatDateFileInfo {
	var (
		min   int64
		max   int64
		err   error
		i     int64
		rows  []StatDateFileInfo
		total StatDateFileInfo
	)
	min = 20190101
	max = 20190101
	for k := range this.statMap.Get() {
		ks := strings.Split(k, "_")
		if len(ks) == 2 {
			if i, err = strconv.ParseInt(ks[0], 10, 64); err != nil {
				continue
			}
			if i >= max {
				max = i
			}
			if i < min {
				min = i
			}
		}
	}
	for i := min; i <= max; i++ {
		s := fmt.Sprintf("%d", i)
		if v, ok := this.statMap.GetValue(s + "_" + CONST_STAT_FILE_TOTAL_SIZE_KEY); ok {
			var info StatDateFileInfo
			info.Date = s
			switch v.(type) {
			case int64:
				info.TotalSize = v.(int64)
				total.TotalSize = total.TotalSize + v.(int64)
			}
			if v, ok := this.statMap.GetValue(s + "_" + CONST_STAT_FILE_COUNT_KEY); ok {
				switch v.(type) {
				case int64:
					info.FileCount = v.(int64)
					total.FileCount = total.FileCount + v.(int64)
				}
			}
			rows = append(rows, info)
		}
	}
	total.Date = "all"
	rows = append(rows, total)
	return rows
}
func (this *Server) RegisterExit() {
	c := make(chan os.Signal)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		for s := range c {
			switch s {
			case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
				this.ldb.Close()
				log.Info("Exit", s)
				os.Exit(1)
			}
		}
	}()
}
func (this *Server) AppendToQueue(fileInfo *FileInfo) {
	this.queueToPeers <- *fileInfo
}
func (this *Server) AppendToDownloadQueue(fileInfo *FileInfo) {
	this.queueFromPeers <- *fileInfo
}
func (this *Server) ConsumerDownLoad() {
	ConsumerFunc := func() {
		for {
			fileInfo := <-this.queueFromPeers
			if len(fileInfo.Peers) <= 0 {
				log.Warn("Peer is null", fileInfo)
				continue
			}
			for _, peer := range fileInfo.Peers {
				if strings.Contains(peer, "127.0.0.1") {
					log.Warn("sync error with 127.0.0.1", fileInfo)
					continue
				}
				if peer != this.host {
					this.DownloadFromPeer(peer, &fileInfo)
					break
				}
			}
		}
	}
	for i := 0; i < 50; i++ {
		go ConsumerFunc()
	}
}
func (this *Server) ConsumerLog() {
	go func() {
		var (
			fileLog *FileLog
		)
		for {
			fileLog = <-this.queueFileLog
			this.saveFileMd5Log(fileLog.FileInfo, fileLog.FileName)
		}
	}()
}
func (this *Server) Consumer() {
	ConsumerFunc := func() {
		for {
			fileInfo := <-this.queueToPeers
			this.postFileToPeer(&fileInfo)
		}
	}
	for i := 0; i < 50; i++ {
		go ConsumerFunc()
	}
}
func (this *Server) AutoRepair(forceRepair bool) {
	if this.lockMap.IsLock("AutoRepair") {
		log.Warn("Lock AutoRepair")
		return
	}
	this.lockMap.LockKey("AutoRepair")
	defer this.lockMap.UnLockKey("AutoRepair")
	AutoRepairFunc := func(forceRepair bool) {
		var (
			dateStats []StatDateFileInfo
			err       error
			countKey  string
			md5s      string
			localSet  mapset.Set
			remoteSet mapset.Set
			allSet    mapset.Set
			tmpSet    mapset.Set
			fileInfo  *FileInfo
		)
		defer func() {
			if re := recover(); re != nil {
				buffer := debug.Stack()
				log.Error("AutoRepair")
				log.Error(re)
				log.Error(string(buffer))
			}
		}()
		Update := func(peer string, dateStat StatDateFileInfo) { //从远端拉数据过来
			req := httplib.Get(fmt.Sprintf("%s%s?date=%s&force=%s", peer, this.getRequestURI("sync"), dateStat.Date, "1"))
			req.SetTimeout(time.Second*5, time.Second*5)
			if _, err = req.String(); err != nil {
				log.Error(err)
			}
			log.Info(fmt.Sprintf("syn file from %s date %s", peer, dateStat.Date))
		}
		for _, peer := range Config().Peers {
			req := httplib.Post(fmt.Sprintf("%s%s", peer, this.getRequestURI("stat")))
			req.Param("inner", "1")
			req.SetTimeout(time.Second*5, time.Second*15)
			if err = req.ToJSON(&dateStats); err != nil {
				log.Error(err)
				continue
			}
			for _, dateStat := range dateStats {
				if dateStat.Date == "all" {
					continue
				}
				countKey = dateStat.Date + "_" + CONST_STAT_FILE_COUNT_KEY
				if v, ok := this.statMap.GetValue(countKey); ok {
					switch v.(type) {
					case int64:
						if v.(int64) != dateStat.FileCount || forceRepair { //不相等,找差异
							//TODO
							req := httplib.Post(fmt.Sprintf("%s%s", peer, this.getRequestURI("get_md5s_by_date")))
							req.SetTimeout(time.Second*15, time.Second*60)
							req.Param("date", dateStat.Date)
							if md5s, err = req.String(); err != nil {
								continue
							}
							if localSet, err = this.GetMd5sByDate(dateStat.Date, CONST_FILE_Md5_FILE_NAME); err != nil {
								log.Error(err)
								continue
							}
							remoteSet = this.util.StrToMapSet(md5s, ",")
							allSet = localSet.Union(remoteSet)
							md5s = this.util.MapSetToStr(allSet.Difference(localSet), ",")
							req = httplib.Post(fmt.Sprintf("%s%s", peer, this.getRequestURI("receive_md5s")))
							req.SetTimeout(time.Second*15, time.Second*60)
							req.Param("md5s", md5s)
							req.String()
							tmpSet = allSet.Difference(remoteSet)
							for v := range tmpSet.Iter() {
								if v != nil {
									if fileInfo, err = this.GetFileInfoFromLevelDB(v.(string)); err != nil {
										log.Error(err)
										continue
									}
									this.AppendToQueue(fileInfo)
								}
							}
							//Update(peer,dateStat)
						}
					}
				} else {
					Update(peer, dateStat)
				}
			}
		}
	}
	AutoRepairFunc(forceRepair)
}
func (this *Server) CleanLogLevelDBByDate(date string, filename string) {
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("CleanLogLevelDBByDate")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	var (
		err       error
		keyPrefix string
		keys      mapset.Set
	)
	keys = mapset.NewSet()
	keyPrefix = "%s_%s_"
	keyPrefix = fmt.Sprintf(keyPrefix, date, filename)
	iter := server.logDB.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
	for iter.Next() {
		keys.Add(string(iter.Value()))
	}
	iter.Release()
	for key := range keys.Iter() {
		err = this.RemoveKeyFromLevelDB(key.(string), this.logDB)
		if err != nil {
			log.Error(err)
		}
	}
}
func (this *Server) CleanAndBackUp() {
	Clean := func() {
		var (
			filenames []string
			yesterday string
		)
		if this.curDate != this.util.GetToDay() {
			filenames = []string{CONST_Md5_QUEUE_FILE_NAME, CONST_Md5_ERROR_FILE_NAME, CONST_REMOME_Md5_FILE_NAME}
			yesterday = this.util.GetDayFromTimeStamp(time.Now().AddDate(0, 0, -1).Unix())
			for _, filename := range filenames {
				this.CleanLogLevelDBByDate(yesterday, filename)
			}
			this.BackUpMetaDataByDate(yesterday)
			this.curDate = this.util.GetToDay()
		}
	}
	go func() {
		for {
			time.Sleep(time.Hour * 6)
			Clean()
		}
	}()
}
func (this *Server) LoadFileInfoByDate(date string, filename string) (mapset.Set, error) {
	defer func() {
		if re := recover(); re != nil {
			buffer := debug.Stack()
			log.Error("LoadFileInfoByDate")
			log.Error(re)
			log.Error(string(buffer))
		}
	}()
	var (
		err       error
		keyPrefix string
		fileInfos mapset.Set
	)
	fileInfos = mapset.NewSet()
	keyPrefix = "%s_%s_"
	keyPrefix = fmt.Sprintf(keyPrefix, date, filename)
	iter := server.logDB.NewIterator(util.BytesPrefix([]byte(keyPrefix)), nil)
	for iter.Next() {
		var fileInfo FileInfo
		if err = json.Unmarshal(iter.Value(), &fileInfo); err != nil {
			continue
		}
		fileInfos.Add(&fileInfo)
	}
	iter.Release()
	return fileInfos, nil
}
func (this *Server) LoadQueue() {
	if queue, err := this.LoadFileInfoByDate(this.util.GetToDay(), CONST_Md5_QUEUE_FILE_NAME); err != nil {
		log.Error(err)
	} else {
		for fileInfo := range queue.Iter() {
			this.queueFromPeers <- *fileInfo.(*FileInfo)
		}
	}
}
func (this *Server) Check() {
	check := func() {
		defer func() {
			if re := recover(); re != nil {
				buffer := debug.Stack()
				log.Error("Check")
				log.Error(re)
				log.Error(string(buffer))
			}
		}()
		var (
			status  JsonResult
			err     error
			subject string
			body    string
			req     *httplib.BeegoHTTPRequest
		)
		for _, peer := range Config().Peers {
			req = httplib.Get(fmt.Sprintf("%s%s", peer, this.getRequestURI("status")))
			req.SetTimeout(time.Second*5, time.Second*5)
			err = req.ToJSON(&status)
			if status.Status != "ok" {
				for _, to := range Config().AlramReceivers {
					subject = "fastdfs server error"
					if err != nil {
						body = fmt.Sprintf("%s\nserver:%s\nerror:\n%s", subject, peer, err.Error())
					} else {
						body = fmt.Sprintf("%s\nserver:%s\n", subject, peer)
					}
					if err = this.SendToMail(to, subject, body, "text"); err != nil {
						log.Error(err)
					}
				}
				if Config().AlarmUrl != "" {
					req = httplib.Post(Config().AlarmUrl)
					req.SetTimeout(time.Second*10, time.Second*10)
					req.Param("message", body)
					req.Param("subject", subject)
					if _, err = req.String(); err != nil {
						log.Error(err)
					}
				}
			}
		}
	}
	go func() {
		for {
			time.Sleep(time.Minute * 10)
			check()
		}
	}()
}
func (this *Server) RepairFileInfo(w http.ResponseWriter, r *http.Request) {
	var (
		result JsonResult
	)
	if !this.IsPeer(r) {
		w.Write([]byte(this.GetClusterNotPermitMessage(r)))
		return
	}
	result.Status = "ok"
	//result.Message = "repair job start,don't try again"
	result.Message = "very danger , disable by system"
	//go this.RepairFileInfoFromFile()
	w.Write([]byte(this.util.JsonEncodePretty(result)))
}
func (this *Server) Reload(w http.ResponseWriter, r *http.Request) {
	var (
		err     error
		data    []byte
		cfg     GloablConfig
		action  string
		cfgjson string
		result  JsonResult
	)
	result.Status = "fail"
	r.ParseForm()
	if !this.IsPeer(r) {
		w.Write([]byte(this.GetClusterNotPermitMessage(r)))
		return
	}
	cfgjson = r.FormValue("cfg")
	action = r.FormValue("action")
	_ = cfgjson
	if action == "get" {
		result.Data = Config()
		result.Status = "ok"
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	if action == "set" {
		if cfgjson == "" {
			result.Message = "(error)parameter cfg(json) require"
			w.Write([]byte(this.util.JsonEncodePretty(result)))
			return
		}
		if err = json.Unmarshal([]byte(cfgjson), &cfg); err != nil {
			log.Error(err)
			result.Message = err.Error()
			w.Write([]byte(this.util.JsonEncodePretty(result)))
			return
		}
		result.Status = "ok"
		cfgjson = this.util.JsonEncodePretty(cfg)
		this.util.WriteFile(CONST_CONF_FILE_NAME, cfgjson)
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	if action == "reload" {
		if data, err = ioutil.ReadFile(CONST_CONF_FILE_NAME); err != nil {
			result.Message = err.Error()
			w.Write([]byte(this.util.JsonEncodePretty(result)))
			return
		}
		if err = json.Unmarshal(data, &cfg); err != nil {
			result.Message = err.Error()
			w.Write([]byte(this.util.JsonEncodePretty(result)))
			return
		}
		ParseConfig(CONST_CONF_FILE_NAME)
		this.initComponent(true)
		result.Status = "ok"
		w.Write([]byte(this.util.JsonEncodePretty(result)))
		return
	}
	if action == "" {
		w.Write([]byte("(error)action support set(json) get reload"))
	}
}
func (this *Server) BackUp(w http.ResponseWriter, r *http.Request) {
	var (
		date   string
		result JsonResult
	)
	result.Status = "ok"
	r.ParseForm()
	date = r.FormValue("date")
	if date == "" {
		date = this.util.GetToDay()
	}
	if this.IsPeer(r) {
		go this.BackUpMetaDataByDate(date)
		result.Message = "back job start..."
		w.Write([]byte(this.util.JsonEncodePretty(result)))
	} else {
		result.Message = this.GetClusterNotPermitMessage(r)
		w.Write([]byte(this.util.JsonEncodePretty(result)))
	}
}
func (this *Server) Repair(w http.ResponseWriter, r *http.Request) {
	var (
		force       string
		forceRepair bool
		result      JsonResult
	)
	result.Status = "ok"
	r.ParseForm()
	force = r.FormValue("force")
	if force == "1" {
		forceRepair = true
	}
	if this.IsPeer(r) {
		go this.AutoRepair(forceRepair)
		result.Message = "repair job start..."
		w.Write([]byte(this.util.JsonEncodePretty(result)))
	} else {
		result.Message = this.GetClusterNotPermitMessage(r)
		w.Write([]byte(this.util.JsonEncodePretty(result)))
	}
}
func (this *Server) Status(w http.ResponseWriter, r *http.Request) {
	var (
		status JsonResult
		err    error
		data   []byte
		sts    map[string]interface{}
		today  string
		sumset mapset.Set
		ok     bool
		v      interface{}
	)
	memStat := new(runtime.MemStats)
	runtime.ReadMemStats(memStat)
	today = this.util.GetToDay()
	sts = make(map[string]interface{})
	sts["Fs.QueueFromPeers"] = len(this.queueFromPeers)
	sts["Fs.QueueToPeers"] = len(this.queueToPeers)
	for _, k := range []string{CONST_FILE_Md5_FILE_NAME, CONST_Md5_ERROR_FILE_NAME, CONST_Md5_QUEUE_FILE_NAME} {
		k2 := fmt.Sprintf("%s_%s", today, k)
		if v, ok = this.sumMap.GetValue(k2); ok {
			sumset = v.(mapset.Set)
			if k == CONST_Md5_QUEUE_FILE_NAME {
				sts["Fs.QueueSetSize"] = sumset.Cardinality()
			}
			if k == CONST_Md5_ERROR_FILE_NAME {
				sts["Fs.ErrorSetSize"] = sumset.Cardinality()
			}
			if k == CONST_FILE_Md5_FILE_NAME {
				sts["Fs.FileSetSize"] = sumset.Cardinality()
			}
		}
	}
	sts["Fs.AutoRepair"] = Config().AutoRepair
	sts["Fs.RefreshInterval"] = Config().RefreshInterval
	sts["Fs.Peers"] = Config().Peers
	sts["Fs.Local"] = this.host
	sts["Fs.FileStats"] = this.GetStat()
	sts["Fs.ShowDir"] = Config().ShowDir
	sts["Sys.NumGoroutine"] = runtime.NumGoroutine()
	sts["Sys.NumCpu"] = runtime.NumCPU()
	sts["Sys.Alloc"] = memStat.Alloc
	sts["Sys.TotalAlloc"] = memStat.TotalAlloc
	sts["Sys.HeapAlloc"] = memStat.HeapAlloc
	sts["Sys.Frees"] = memStat.Frees
	sts["Sys.HeapObjects"] = memStat.HeapObjects
	sts["Sys.NumGC"] = memStat.NumGC
	sts["Sys.GCCPUFraction"] = memStat.GCCPUFraction
	sts["Sys.GCSys"] = memStat.GCSys
	//sts["Sys.MemInfo"] = memStat
	status.Status = "ok"
	status.Data = sts
	w.Write([]byte(this.util.JsonEncodePretty(status)))
	return
	if data, err = json.Marshal(&status); err != nil {
		status.Status = "fail"
		status.Message = err.Error()
		w.Write(data)
		return
	}
	w.Write(data)
}
func (this *Server) HeartBeat(w http.ResponseWriter, r *http.Request) {
}
func (this *Server) Index(w http.ResponseWriter, r *http.Request) {
	var (
		uploadUrl    string
		uploadBigUrl string
		uppy         string
	)
	uploadUrl = "/upload"
	uploadBigUrl = CONST_BIG_UPLOAD_PATH_SUFFIX
	if Config().EnableWebUpload {
		if Config().SupportGroupManage {
			uploadUrl = fmt.Sprintf("/%s/upload", Config().Group)
			uploadBigUrl = fmt.Sprintf("/%s%s", Config().Group, CONST_BIG_UPLOAD_PATH_SUFFIX)
		}
		uppy = `<html>
			  
			  <head>
				<meta charset="utf-8" />
				<title>Uploader</title>
				<style>form { bargin } .form-line { display:block;height: 30px;margin:8px; } #stdUpload {background: #fafafa;border-radius: 10px;width: 745px; }</style>
				<link href="https://transloadit.edgly.net/releases/uppy/v0.30.0/dist/uppy.min.css" rel="stylesheet"></head>
			  
			  <body>
                <div>标准上传(强列建议使用这种方式)</div>
				<div id="stdUpload">
				  
				  <form action="%s" method="post" enctype="multipart/form-data">
					<span class="form-line">文件(file):
					  <input type="file" id="file" name="file" /></span>
					<span class="form-line">场景(scene):
					  <input type="text" id="scene" name="scene" value="%s" /></span>
					<span class="form-line">输出(output):
					  <input type="text" id="output" name="output" value="json" /></span>
					<span class="form-line">自定义路径(path):
					  <input type="text" id="path" name="path" value="" /></span>
					<input type="submit" name="submit" value="upload" /></form>
				</div>
                 <div>断点续传（如果文件很大时可以考虑）</div>
				<div>
				 
				  <div id="drag-drop-area"></div>
				  <script src="https://transloadit.edgly.net/releases/uppy/v0.30.0/dist/uppy.min.js"></script>
				  <script>var uppy = Uppy.Core().use(Uppy.Dashboard, {
					  inline: true,
					  target: '#drag-drop-area'
					}).use(Uppy.Tus, {
					  endpoint: '%s'
					})
					uppy.on('complete', (result) => {
					 // console.log(result) console.log('Upload complete! We’ve uploaded these files:', result.successful)
					})
                </script>
				</div>
			  </body>
			</html>`
		fmt.Fprintf(w,
			fmt.Sprintf(uppy, uploadUrl, Config().DefaultScene, uploadBigUrl))
	} else {
		w.Write([]byte("web upload deny"))
	}
}
func init() {
	DOCKER_DIR = os.Getenv("GO_FASTDFS_DIR")
	if DOCKER_DIR != "" {
		if !strings.HasSuffix(DOCKER_DIR, "/") {
			DOCKER_DIR = DOCKER_DIR + "/"
		}
	}
	STORE_DIR = DOCKER_DIR + STORE_DIR_NAME
	CONF_DIR = DOCKER_DIR + CONF_DIR_NAME
	DATA_DIR = DOCKER_DIR + DATA_DIR_NAME
	LOG_DIR = DOCKER_DIR + LOG_DIR_NAME
	LARGE_DIR_NAME = "haystack"
	LARGE_DIR = STORE_DIR + "/haystack"
	CONST_LEVELDB_FILE_NAME = DATA_DIR + "/fileserver.db"
	CONST_LOG_LEVELDB_FILE_NAME = DATA_DIR + "/log.db"
	CONST_STAT_FILE_NAME = DATA_DIR + "/stat.json"
	CONST_CONF_FILE_NAME = CONF_DIR + "/cfg.json"
	FOLDERS = []string{DATA_DIR, STORE_DIR, CONF_DIR}
	logAccessConfigStr = strings.Replace(logAccessConfigStr, "{DOCKER_DIR}", DOCKER_DIR, -1)
	logConfigStr = strings.Replace(logConfigStr, "{DOCKER_DIR}", DOCKER_DIR, -1)
	for _, folder := range FOLDERS {
		os.MkdirAll(folder, 0775)
	}
	server = NewServer()
	flag.Parse()
	peerId := fmt.Sprintf("%d", server.util.RandInt(0, 9))
	if !server.util.FileExists(CONST_CONF_FILE_NAME) {
		peer := "http://" + server.util.GetPulicIP() + ":8080"
		cfg := fmt.Sprintf(cfgJson, peerId, peer, peer)
		server.util.WriteFile(CONST_CONF_FILE_NAME, cfg)
	}
	if logger, err := log.LoggerFromConfigAsBytes([]byte(logConfigStr)); err != nil {
		panic(err)
	} else {
		log.ReplaceLogger(logger)
	}
	if _logacc, err := log.LoggerFromConfigAsBytes([]byte(logAccessConfigStr)); err == nil {
		logacc = _logacc
		log.Info("succes init log access")
	} else {
		log.Error(err.Error())
	}
	ParseConfig(CONST_CONF_FILE_NAME)
	if Config().QueueSize == 0 {
		Config().QueueSize = CONST_QUEUE_SIZE
	}
	if Config().PeerId == "" {
		Config().PeerId = peerId
	}
	staticHandler = http.StripPrefix("/"+Config().Group+"/", http.FileServer(http.Dir(STORE_DIR)))
	server.initComponent(false)
}
func (this *Server) test() {
	testLock := func() {
		tt := func(i int) {
			if server.lockMap.IsLock("xx") {
				return
			}
			server.lockMap.LockKey("xx")
			defer server.lockMap.UnLockKey("xx")
			//time.Sleep(time.Nanosecond*1)
			fmt.Println("xx", i)
		}
		for i := 0; i < 10000; i++ {
			go tt(i)
		}
		time.Sleep(time.Second * 3)
		go tt(999999)
		go tt(999999)
		go tt(999999)
	}
	_ = testLock
	testFile := func() {
		var (
			err error
			f   *os.File
		)
		f, err = os.OpenFile("tt", os.O_CREATE|os.O_RDWR, 0777)
		if err != nil {
			fmt.Println(err)
		}
		f.WriteAt([]byte("1"), 100)
		f.Seek(0, 2)
		f.Write([]byte("2"))
		//fmt.Println(f.Seek(0, 2))
		//fmt.Println(f.Seek(3, 2))
		//fmt.Println(f.Seek(3, 0))
		//fmt.Println(f.Seek(3, 1))
		//fmt.Println(f.Seek(3, 0))
		//f.Write([]byte("1"))
	}
	_ = testFile
	//testFile()
}
func (this *Server) initTus() {
	var (
		err     error
		fileLog *os.File
		bigDir  string
	)
	BIG_DIR := STORE_DIR + "/_big/" + Config().PeerId
	os.MkdirAll(BIG_DIR, 0775)
	os.MkdirAll(LOG_DIR, 0775)
	store := filestore.FileStore{
		Path: BIG_DIR,
	}
	if fileLog, err = os.OpenFile(LOG_DIR+"/tusd.log", os.O_CREATE|os.O_RDWR, 0666); err != nil {
		log.Error(err)
		panic("initTus")
	}
	go func() {
		for {
			if fi, err := fileLog.Stat(); err != nil {
				log.Error(err)
			} else {
				if fi.Size() > 1024*1024*500 { //500M
					this.util.CopyFile(LOG_DIR+"/tusd.log", LOG_DIR+"/tusd.log.2")
					fileLog.Seek(0, 0)
					fileLog.Truncate(0)
					fileLog.Seek(0, 2)
				}
			}
			time.Sleep(time.Second * 30)
		}
	}()
	l := slog.New(fileLog, "[tusd] ", slog.LstdFlags)
	bigDir = CONST_BIG_UPLOAD_PATH_SUFFIX
	if Config().SupportGroupManage {
		bigDir = fmt.Sprintf("/%s%s", Config().Group, CONST_BIG_UPLOAD_PATH_SUFFIX)
	}
	composer := tusd.NewStoreComposer()
	// support raw tus upload and download
	store.GetReaderExt = func(id string) (io.Reader, error) {
		var (
			offset int64
			err    error
			length int
			buffer []byte
			fi     *FileInfo
		)
		if fi, err = this.GetFileInfoFromLevelDB(id); err != nil {
			log.Error(err)
			return nil, err
		} else {
			fp := DOCKER_DIR + fi.Path + "/" + fi.ReName
			if this.util.FileExists(fp) {
				log.Info(fmt.Sprintf("download:%s", fp))
				return os.Open(fp)
			}
			ps := strings.Split(fp, ",")
			if len(ps) > 2 && this.util.FileExists(ps[0]) {
				if length, err = strconv.Atoi(ps[2]); err != nil {
					return nil, err
				}
				if offset, err = strconv.ParseInt(ps[1], 10, 64); err != nil {
					return nil, err
				}
				if buffer, err = this.util.ReadFileByOffSet(ps[0], offset, length); err != nil {
					return nil, err
				}
				if buffer[0] == '1' {
					bufferReader := bytes.NewBuffer(buffer[1:])
					return bufferReader, nil
				} else {
					msg := "data no sync"
					log.Error(msg)
					return nil, errors.New(msg)
				}
			}
			return nil, errors.New(fmt.Sprintf("%s not found", fp))
		}
	}
	store.UseIn(composer)
	handler, err := tusd.NewHandler(tusd.Config{
		Logger:                l,
		BasePath:              bigDir,
		StoreComposer:         composer,
		NotifyCompleteUploads: true,
	})
	notify := func(handler *tusd.Handler) {
		for {
			select {
			case info := <-handler.CompleteUploads:
				log.Info("CompleteUploads", info)
				name := ""
				if v, ok := info.MetaData["filename"]; ok {
					name = v
				}
				var err error
				md5sum := ""
				oldFullPath := BIG_DIR + "/" + info.ID + ".bin"
				infoFullPath := BIG_DIR + "/" + info.ID + ".info"
				if md5sum, err = this.util.GetFileSumByName(oldFullPath, Config().FileSumArithmetic); err != nil {
					log.Error(err)
					continue
				}
				ext := path.Ext(name)
				filename := md5sum + ext
				timeStamp := time.Now().Unix()
				fpath := time.Now().Format("/20060102/15/04/")
				newFullPath := STORE_DIR + "/" + Config().DefaultScene + fpath + Config().PeerId + "/" + filename
				if fi, err := this.GetFileInfoFromLevelDB(md5sum); err != nil {
					log.Error(err)
				} else {
					if fi.Md5 != "" {
						if _, err := this.SaveFileInfoToLevelDB(info.ID, fi, this.ldb); err != nil {
							log.Error(err)
						}
						log.Info(fmt.Sprintf("file is found md5:%s", fi.Md5))
						log.Info("remove file:", oldFullPath)
						log.Info("remove file:", infoFullPath)
						os.Remove(oldFullPath)
						os.Remove(infoFullPath)
						continue
					}
				}
				fpath = STORE_DIR_NAME + "/" + Config().DefaultScene + fpath + Config().PeerId
				os.MkdirAll(DOCKER_DIR+fpath, 0775)
				fileInfo := &FileInfo{
					Name:      name,
					Path:      fpath,
					ReName:    filename,
					Size:      info.Size,
					TimeStamp: timeStamp,
					Md5:       md5sum,
					Peers:     []string{this.host},
					OffSet:    -1,
				}
				if err = os.Rename(oldFullPath, newFullPath); err != nil {
					log.Error(err)
					continue
				}
				os.Remove(infoFullPath)
				if _, err = this.SaveFileInfoToLevelDB(info.ID, fileInfo, this.ldb); err != nil { //assosiate file id
					log.Error(err)
				}
				this.SaveFileMd5Log(fileInfo, CONST_FILE_Md5_FILE_NAME)
				go this.postFileToPeer(fileInfo)
			}
		}
	}
	go notify(handler)
	if err != nil {
		log.Error(err)
	}
	http.Handle(bigDir, http.StripPrefix(bigDir, handler))
}
func (this *Server) FormatStatInfo() {
	var (
		data  []byte
		err   error
		count int64
		stat  map[string]interface{}
	)
	if this.util.FileExists(CONST_STAT_FILE_NAME) {
		if data, err = this.util.ReadBinFile(CONST_STAT_FILE_NAME); err != nil {
			log.Error(err)
		} else {
			if err = json.Unmarshal(data, &stat); err != nil {
				log.Error(err)
			} else {
				for k, v := range stat {
					switch v.(type) {
					case float64:
						vv := strings.Split(fmt.Sprintf("%f", v), ".")[0]
						if count, err = strconv.ParseInt(vv, 10, 64); err != nil {
							log.Error(err)
						} else {
							this.statMap.Put(k, count)
						}
					default:
						this.statMap.Put(k, v)
					}
				}
			}
		}
	} else {
		this.RepairStatByDate(this.util.GetToDay())
	}
}
func (this *Server) initComponent(isReload bool) {
	var (
		ip string
	)
	ip = this.util.GetPulicIP()
	if Config().Host == "" {
		if len(strings.Split(Config().Addr, ":")) == 2 {
			server.host = fmt.Sprintf("http://%s:%s", ip, strings.Split(Config().Addr, ":")[1])
			Config().Host = server.host
		}
	} else {
		server.host = Config().Host
	}
	ex, _ := regexp.Compile("\\d+\\.\\d+\\.\\d+\\.\\d+")
	var peers []string
	for _, peer := range Config().Peers {
		if this.util.Contains(ip, ex.FindAllString(peer, -1)) ||
			this.util.Contains("127.0.0.1", ex.FindAllString(peer, -1)) {
			continue
		}
		if strings.HasPrefix(peer, "http") {
			peers = append(peers, peer)
		} else {
			peers = append(peers, "http://"+peer)
		}
	}
	Config().Peers = peers
	if !isReload {
		this.FormatStatInfo()
		this.initTus()
	}
	//Timer
	// int tus
}

type HttpHandler struct {
}

func (HttpHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	status_code := "200"
	defer func(t time.Time) {
		logStr := fmt.Sprintf("[Access] %s | %v | %s | %s | %s | %s |%s",
			time.Now().Format("2006/01/02 - 15:04:05"),
			res.Header(),
			time.Since(t).String(),
			server.util.GetClientIp(req),
			req.Method,
			status_code,
			req.RequestURI,
		)
		logacc.Info(logStr)
	}(time.Now())
	defer func() {
		if err := recover(); err != nil {
			status_code = "500"
			res.WriteHeader(500)
			print(err)
			buff := debug.Stack()
			log.Error(err)
			log.Error(string(buff))
		}
	}()
	http.DefaultServeMux.ServeHTTP(res, req)
}
func (this *Server) Main() {
	go func() {
		for {
			this.CheckFileAndSendToPeer(this.util.GetToDay(), CONST_Md5_ERROR_FILE_NAME, false)
			//fmt.Println("CheckFileAndSendToPeer")
			time.Sleep(time.Second * time.Duration(Config().RefreshInterval))
			//this.util.RemoveEmptyDir(STORE_DIR)
		}
	}()
	go this.CleanAndBackUp()
	go this.Check()
	go this.LoadQueue()
	go this.Consumer()
	go this.ConsumerLog()
	go this.ConsumerDownLoad()
	if Config().AutoRepair {
		go func() {
			for {
				time.Sleep(time.Minute * 3)
				this.AutoRepair(false)
				time.Sleep(time.Minute * 60)
			}
		}()
	}
	if Config().SupportGroupManage {
		http.HandleFunc(fmt.Sprintf("/%s", Config().Group), this.Index)
		http.HandleFunc(fmt.Sprintf("/%s/check_file_exist", Config().Group), this.CheckFileExist)
		http.HandleFunc(fmt.Sprintf("/%s/upload", Config().Group), this.Upload)
		http.HandleFunc(fmt.Sprintf("/%s/delete", Config().Group), this.RemoveFile)
		http.HandleFunc(fmt.Sprintf("/%s/sync", Config().Group), this.Sync)
		http.HandleFunc(fmt.Sprintf("/%s/stat", Config().Group), this.Stat)
		http.HandleFunc(fmt.Sprintf("/%s/repair_stat", Config().Group), this.RepairStatWeb)
		http.HandleFunc(fmt.Sprintf("/%s/status", Config().Group), this.Status)
		http.HandleFunc(fmt.Sprintf("/%s/repair", Config().Group), this.Repair)
		http.HandleFunc(fmt.Sprintf("/%s/BackUp", Config().Group), this.BackUp)
		http.HandleFunc(fmt.Sprintf("/%s/repair_fileinfo", Config().Group), this.RepairFileInfo)
		http.HandleFunc(fmt.Sprintf("/%s/reload", Config().Group), this.Reload)
		http.HandleFunc(fmt.Sprintf("/%s/syncfile_info", Config().Group), this.SyncFileInfo)
		http.HandleFunc(fmt.Sprintf("/%s/get_md5s_by_date", Config().Group), this.GetMd5sForWeb)
		http.HandleFunc(fmt.Sprintf("/%s/receive_md5s", Config().Group), this.ReceiveMd5s)
	} else {
		http.HandleFunc("/", this.Index)
		http.HandleFunc("/check_file_exist", this.CheckFileExist)
		http.HandleFunc("/upload", this.Upload)
		http.HandleFunc("/delete", this.RemoveFile)
		http.HandleFunc("/sync", this.Sync)
		http.HandleFunc("/stat", this.Stat)
		http.HandleFunc("/repair_stat", this.RepairStatWeb)
		http.HandleFunc("/status", this.Status)
		http.HandleFunc("/repair", this.Repair)
		http.HandleFunc("/backup", this.BackUp)
		http.HandleFunc("/repair_fileinfo", this.RepairFileInfo)
		http.HandleFunc("/reload", this.Reload)
		http.HandleFunc("/syncfile_info", this.SyncFileInfo)
		http.HandleFunc("/get_md5s_by_date", this.GetMd5sForWeb)
		http.HandleFunc("/receive_md5s", this.ReceiveMd5s)
	}
	http.HandleFunc("/"+Config().Group+"/", this.Download)
	fmt.Println("Listen on " + Config().Addr)
	err := http.ListenAndServe(Config().Addr, new(HttpHandler))
	log.Error(err)
	fmt.Println(err)
}
func main() {
	server.Main()
}
