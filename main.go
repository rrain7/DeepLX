package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/abadojack/whatlanggo"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"golang.org/x/time/rate"
)

func init() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

type Lang struct {
	SourceLangUserSelected string `json:"source_lang_user_selected"`
	TargetLang             string `json:"target_lang"`
}

type CommonJobParams struct {
	WasSpoken    bool   `json:"wasSpoken"`
	TranscribeAS string `json:"transcribe_as"`
	// RegionalVariant string `json:"regionalVariant"`
}

type Params struct {
	Texts           []Text          `json:"texts"`
	Splitting       string          `json:"splitting"`
	Lang            Lang            `json:"lang"`
	Timestamp       int64           `json:"timestamp"`
	CommonJobParams CommonJobParams `json:"commonJobParams"`
}

type Text struct {
	Text                string `json:"text"`
	RequestAlternatives int    `json:"requestAlternatives"`
}

type PostData struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	ID      int64  `json:"id"`
	Params  Params `json:"params"`
}

func initData(sourceLang string, targetLang string) *PostData {
	return &PostData{
		Jsonrpc: "2.0",
		Method:  "LMT_handle_texts",
		Params: Params{
			Splitting: "newlines",
			Lang: Lang{
				SourceLangUserSelected: sourceLang,
				TargetLang:             targetLang,
			},
			CommonJobParams: CommonJobParams{
				WasSpoken:    false,
				TranscribeAS: "",
				// RegionalVariant: "en-US",
			},
		},
	}
}

func getICount(translateText string) int64 {
	return int64(strings.Count(translateText, "i"))
}

func getRandomNumber() int64 {
	rand.Seed(time.Now().Unix())
	num := rand.Int63n(99999) + 8300000
	return num * 1000
}

func getTimeStamp(iCount int64) int64 {
	ts := time.Now().UnixMilli()
	if iCount != 0 {
		iCount = iCount + 1
		return ts - ts%iCount + iCount
	} else {
		return ts
	}
}

type ResData struct {
	TransText  string `json:"text"`
	SourceLang string `json:"source_lang"`
	TargetLang string `json:"target_lang"`
}

var limiter = rate.NewLimiter(forEvery(12, time.Minute), 12)

func forEvery(eventCount int, duration time.Duration) rate.Limit {
	return rate.Every(duration / time.Duration(eventCount))
}

var client = &http.Client{
	Transport: &rateLimitedTransport{
		rt: http.DefaultTransport,
		rl: limiter,
	},
}

func main() {
	// display information
	fmt.Println("DeepL X has been successfully launched! Listening on 0.0.0.0:1188")
	fmt.Println("Made by sjlleo and missuo.")

	// create a random id
	id := getRandomNumber()

	// set release mode
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(cors.Default())

	r.GET("/", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"code":    200,
			"message": "DeepL Free API, Made by sjlleo and missuo. Go to /translate with POST. http://github.com/OwO-Network/DeepLX",
		})

	})

	r.POST("/translate", func(c *gin.Context) {
		reqj := ResData{}
		c.BindJSON(&reqj)

		sourceLang := reqj.SourceLang
		targetLang := reqj.TargetLang
		translateText := reqj.TransText
		if sourceLang == "" {
			lang := whatlanggo.DetectLang(translateText)
			deepLLang := strings.ToUpper(lang.Iso6391())
			sourceLang = deepLLang
		}
		if targetLang == "" {
			targetLang = "EN"
		}
		if translateText == "" {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    http.StatusNotFound,
				"message": "No Translate Text Found",
			})
		} else {
			url := "https://www2.deepl.com/jsonrpc"
			id = id + 1
			postData := initData(sourceLang, targetLang)
			text := Text{
				Text:                translateText,
				RequestAlternatives: 3,
			}
			// set id
			postData.ID = id
			// set text
			postData.Params.Texts = append(postData.Params.Texts, text)
			// set timestamp
			postData.Params.Timestamp = getTimeStamp(getICount(translateText))
			post_byte, _ := json.Marshal(postData)
			postStr := string(post_byte)

			// add space if necessary
			if (id+5)%29 == 0 || (id+3)%13 == 0 {
				postStr = strings.Replace(postStr, "\"method\":\"", "\"method\" : \"", -1)
			} else {
				postStr = strings.Replace(postStr, "\"method\":\"", "\"method\": \"", -1)
			}

			post_byte = []byte(postStr)
			reader := bytes.NewReader(post_byte)
			request, err := http.NewRequest("POST", url, reader)
			if err != nil {
				log.Println(err)
				return
			}

			// Set Headers
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "*/*")
			request.Header.Set("x-app-os-name", "iOS")
			request.Header.Set("x-app-os-version", "16.3.0")
			request.Header.Set("Accept-Language", "en-US,en;q=0.9")
			request.Header.Set("Accept-Encoding", "gzip, deflate, br")
			request.Header.Set("x-app-device", "iPhone13,2")
			request.Header.Set("User-Agent", "DeepL-iOS/2.6.0 iOS 16.3.0 (iPhone13,2)")
			request.Header.Set("x-app-build", "353933")
			request.Header.Set("x-app-version", "2.6")
			request.Header.Set("Connection", "keep-alive")

			resp, err := rateLimitRequest(request)
			if err != nil {
				log.Println("send translate request error: ", err)
				c.JSON(http.StatusTooManyRequests, gin.H{
					"code":    http.StatusTooManyRequests,
					"message": "rate limit exceeded",
				})
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			res := gjson.ParseBytes(body)

			// display response
			// fmt.Println(res)
			if res.Get("error.code").String() == "-32600" {
				log.Println(res.Get("error").String())
				c.JSON(http.StatusNotAcceptable, gin.H{
					"code":    http.StatusNotAcceptable,
					"message": "Invalid targetLang",
				})
				return
			}

			if resp.StatusCode == http.StatusTooManyRequests {
				c.JSON(http.StatusTooManyRequests, gin.H{
					"code":    http.StatusTooManyRequests,
					"message": "Too Many Requests",
				})
			} else {
				var alternatives []string
				res.Get("result.texts.0.alternatives").ForEach(func(key, value gjson.Result) bool {
					alternatives = append(alternatives, value.Get("text").String())
					return true
				})
				c.JSON(http.StatusOK, gin.H{
					"code":         http.StatusOK,
					"id":           id,
					"data":         res.Get("result.texts.0.text").String(),
					"alternatives": alternatives,
				})
			}
		}
	})
	r.Run(":1188") // listen and serve on 0.0.0.0:1188
}

type rateLimitedTransport struct {
	rt http.RoundTripper
	rl *rate.Limiter
}

// rateLimitRequest 限制请求发送
func rateLimitRequest(request *http.Request) (*http.Response, error) {
	limiter.Wait(context.Background())

	resp, err := client.Do(request)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// wait for the limiter to allow the request
	ctx := req.Context()
	err := t.rl.Wait(ctx)
	if err != nil {
		return nil, err
	}

	// make the HTTP request using the underlying transport
	return t.rt.RoundTrip(req)
}
