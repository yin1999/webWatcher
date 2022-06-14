package main

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type handler interface {
	Next() (payload string, reqID string, err error)
	ReportError(msg string, id string)
	ReportSuccess(id string)
	ossMethod
}

type ossMethod interface {
	PutObject(objectName string, data []byte) error
	GetObject(objectName string) ([]byte, error)
}

var (
	client = http.Client{
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
	fileService ossMethod
)

func init() {
	port := os.Getenv("SMTP_PORT")
	emailCfg.SMTP.Port, _ = strconv.Atoi(port)
}

func main() {
	handler, err := regist()
	if err != nil {
		panic(err)
	}
	startServe(handler)
}

func task(url string, fileService ossMethod) error {
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
	oldHash, err = fileService.GetObject(b64)
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
		if err = fileService.PutObject(b64, newHash); err != nil {
			e = append(e, err)
		}
		if len(e) > 0 {
			return fmt.Errorf("%v", e)
		}
	}
	return err
}

func notify(url string) error {
	return emailCfg.Send("web watcher", "网站更新提醒", fmt.Sprintf("网站地址: %s\n", url))
}

func startServe(h handler) error {
	for {
		payload, reqID, err := h.Next()
		if err != nil {
			msg := "parse request failed, err: %s" + err.Error()
			Error.Log(msg)
			h.ReportError(msg, reqID)
			continue
		}
		err = task(payload, h)
		if err != nil {
			h.ReportError(err.Error()+"\n", reqID)
		} else {
			h.ReportSuccess(reqID)
		}
	}
}
