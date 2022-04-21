package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	cos "github.com/tencentyun/cos-go-sdk-v5"
)

const contentType = "text/plain"

var (
	reportAPI string
	cosClient *cos.Client
	client    = http.Client{
		Timeout: 10 * time.Second,
	}
	emailCfg = Config{
		To: strings.Split(os.Getenv("EMAIL_TO"), ","),
		SMTP: SmtpConfig{
			Host:     os.Getenv("SMTP_HOST"),
			TLS:      os.Getenv("SMTP_TLS") == "true",
			Username: os.Getenv("SMTP_USERNAME"),
			Password: os.Getenv("SMTP_PASSWORD"),
		},
	}
)

func init() {
	port := os.Getenv("SMTP_PORT")
	emailCfg.SMTP.Port, _ = strconv.Atoi(port)
}

func main() {
	u, err := url.Parse(os.Getenv("BUCKET_URL"))
	if err != nil {
		panic(err)
	}
	cosClient = cos.NewClient(&cos.BaseURL{BucketURL: u}, &http.Client{
		Timeout: 10 * time.Second,
		Transport: &cos.AuthorizationTransport{
			SecretID:  os.Getenv("COS_SECRETID"),
			SecretKey: os.Getenv("COS_SECRETKEY"),
		},
	})
	host := os.Getenv("SCF_RUNTIME_API")
	port := os.Getenv("SCF_RUNTIME_API_PORT")
	reportAPI = "http://" + host + ":" + port
	res, err := http.Post(reportAPI+"/runtime/init/ready", contentType, http.NoBody)
	if err != nil {
		panic(err)
	}
	res.Body.Close()
	ListenAndServe(handler)
}

type timerTrigger struct {
	TriggerTime string `json:"Time"`
	TriggerName string `json:"TriggerName"`
	Payload     string `json:"Message"`
}

func handler(url string) error {
	h := md5.New()
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	_, err = io.Copy(h, resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	var oldHash []byte
	b64 := base64.URLEncoding.EncodeToString([]byte(url))
	oldHash, err = getFile(b64)
	if err != nil {
		return err
	}
	newHash := h.Sum(nil)
	if !bytes.Equal(newHash, oldHash) {
		e := []error{}
		if len(oldHash) > 0 { // notify only when oldHash is not empty
			if err = notify(url); err != nil {
				e = append(e, err)
			}
		}
		if err = putFile(b64, newHash); err != nil {
			e = append(e, err)
		}
		if len(e) > 0 {
			return fmt.Errorf("%v", e)
		}
	}
	return err
}

func putFile(name string, data []byte) error {
	_, err := cosClient.Object.Put(context.Background(), name, bytes.NewReader(data), nil)
	return err
}

func getFile(name string) (data []byte, err error) {
	var isExist bool
	isExist, err = cosClient.Object.IsExist(context.Background(), name)
	if err != nil || !isExist {
		return
	}
	var res *cos.Response
	res, err = cosClient.Object.Get(context.Background(), name, nil)
	if err != nil {
		return
	}
	defer res.Body.Close()
	data = make([]byte, res.ContentLength)
	var n int
	n, err = res.Body.Read(data)
	if err == io.EOF {
		err = nil
	}
	data = data[:n]
	return
}

func notify(url string) error {
	return emailCfg.Send("web watcher", "网站更新提醒", fmt.Sprintf("网站地址: %s\n", url))
}

func ListenAndServe(handler func(payload string) error) error {
	for {
		res, err := http.Get(reportAPI + "/runtime/invocation/next")
		if err != nil {
			Error.Log("get trigger payload failed, err: %s\n", err.Error())
			reportError("get payload failed, err: " + err.Error() + "\n")
		}
		requestId := res.Header.Get("Request_id")
		dec := json.NewDecoder(res.Body)
		t := &timerTrigger{}
		err = dec.Decode(t)
		res.Body.Close() // close body
		if err != nil {
			msg := "parse request body failed, err: " + err.Error()
			Error.Log(msg + "\n")
			reportError(msg)
		}
		err = handler(t.Payload)
		if err != nil {
			reportError(err.Error() + "\n")
		} else {
			res, err := http.DefaultClient.Post(reportAPI+"/runtime/invocation/response", contentType, strings.NewReader(requestId))
			if err == nil {
				res.Body.Close()
			} else {
				Fatal.Log(err.Error() + "\n")
			}
		}
	}
}

func reportError(msg string) {
	res, err := http.DefaultClient.Post(reportAPI+"/runtime/invocation/error",
		contentType,
		strings.NewReader(msg),
	)
	if err == nil {
		res.Body.Close()
	} else {
		Error.Log(err.Error() + "\n")
	}
}
