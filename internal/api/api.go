package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/imroc/req/v3"
)

type Api struct {
	domain string
	client *req.Client
}

func NewAPI(domain string, token string, proxy func(*http.Request) (*url.URL, error)) *Api {
	return &Api{
		domain: domain,
		client: req.C().
			SetProxy(proxy).
			SetCommonBearerAuthToken(token).
			SetUserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0"),
	}
}

type ListConversationsRequest struct {
	Keywords   string   `json:"keywords"`
	ModelNames []string `json:"model_names"`
	PageNum    int64    `json:"page_num"`
	PageSize   int64    `json:"page_size"`
}

type ListConversationsResponse struct {
	Total int64                     `json:"total"`
	Msg   string                    `json:"msg"`
	Data  []*SimpleConversationInfo `json:"data"`
}

type SimpleConversationInfo struct {
	ID                int64  `json:"id"`
	ConversationID    string `json:"conversation_id"`
	ModelName         string `json:"model_name"`
	ConversationTitle string `json:"conversation_title"`
	AppName           string `json:"app_name"`
	IsCollect         int64  `json:"is_collect"`
	ParentMessageID   string `json:"parent_message_id"`
	Platform          int64  `json:"platform"`
	UID               string `json:"uid"`
	CreateTime        string `json:"create_time"`
	CreateTimestamp   int64  `json:"create_timestamp"`
}

func (api *Api) ListConversations(ctx context.Context, req *ListConversationsRequest) (*ListConversationsResponse, error) {
	urlStr := fmt.Sprintf("%s/api/chat/chatHistory", api.domain)
	httpResp, err := api.client.R().
		SetContext(ctx).
		SetBodyJsonMarshal(req).
		SetSuccessResult(ListConversationsResponse{}).
		Post(urlStr)
	if err != nil {
		return nil, err
	} else if httpResp.GetStatusCode() != http.StatusOK {
		return nil, fmt.Errorf("http error: code=%d, status=%s", httpResp.GetStatusCode(), httpResp.String())
	}
	return httpResp.SuccessResult().(*ListConversationsResponse), nil
}

type ConversationInfoRequest struct {
	ConversationID string `json:"conversation_id"`
	IsV2           bool   `json:"isV2"`    // true
	WebUrl         string `json:"web_url"` // https://api.chandler.bet
}

type ConversationInfoResponse struct {
	Data []*ConversationInfo `json:"data"`
	Msg  string              `json:"msg"`
}

type ConversationInfo struct {
	ModelName   string               `json:"model_name"`
	AppName     string               `json:"app_name"`
	MessageID   string               `json:"message_id"`
	QuestionLen int64                `json:"question_len"`
	QAS         []*QuestionAndAnswer `json:"qas"`
	CreateTime  string               `json:"create_time"`
	UpdateTime  string               `json:"update_time"`
}

type QuestionAndAnswer struct {
	AnswerLen    int64    `json:"answer_len"`
	Question     string   `json:"question"`
	CreateTime   string   `json:"question_time"`
	Answers      []string `json:"answers"`
	QuestionInfo *struct {
		Name  string `json:"question"`
		Image string `json:"imageSrc"`
	} `json:"questionInfo"`
	AnswersInfo [][]*struct {
		Content   string `json:"content"`
		MessageID string `json:"messageID"`
		Type      string `json:"type"` // text
	} `json:"answersInfo"`
}

func (api *Api) ConversationInfo(ctx context.Context, req *ConversationInfoRequest) (*ConversationInfoResponse, error) {
	urlStr := fmt.Sprintf("%s/api/chat/conversationInfo", api.domain)
	httpResp, err := api.client.R().
		SetContext(ctx).
		SetBodyJsonMarshal(req).
		SetSuccessResult(ConversationInfoResponse{}).
		Post(urlStr)
	if err != nil {
		return nil, err
	} else if httpResp.GetStatusCode() != http.StatusOK {
		return nil, fmt.Errorf("http error: code=%d, status=%s", httpResp.GetStatusCode(), httpResp.String())
	}
	return httpResp.SuccessResult().(*ConversationInfoResponse), nil
}

type RenameConversationRequest struct {
	ConversationID       string `json:"conversation_id"`
	IsCollect            int64  `json:"is_collect"` // 0
	NewConversationTitle string `json:"new_conversation_title"`
}

func (api *Api) RenameConversation(ctx context.Context, req *RenameConversationRequest) error {
	urlStr := fmt.Sprintf("%s/api/chat/updateConversation", api.domain)
	httpResp, err := api.client.R().
		SetContext(ctx).
		SetBodyJsonMarshal(req).
		Post(urlStr)
	if err != nil {
		return err
	} else if httpResp.GetStatusCode() != http.StatusOK {
		return fmt.Errorf("http error: code=%d, status=%s", httpResp.GetStatusCode(), httpResp.String())
	}
	return nil
}

type ChatConversationRequest struct {
	AiReply         string `json:"aireply"`      // ""
	AnswerAgain     bool   `json:"answer_again"` // false
	AppName         string `json:"app_name"`
	AttachmentList  []any  `json:"attachment_list"` // []
	ConversationID  string `json:"conversation_id"`
	GlobalTimeout   int64  `json:"global_timeout"` // 100
	MaxRetries      int64  `json:"max_retries"`    // 1
	ModelName       string `json:"model_name"`
	ParentMessageID string `json:"parent_message_id"`
	Prompt          string `json:"prompt"`
	RequestTimeout  int64  `json:"request_timeout"` // 30
	Status          string `json:"status"`          // ""
	Timestamp       int64  `json:"timestamp"`       // ms
	UID             string `json:"uid"`
	WebUrl          string `json:"web_url"`
}

type ChatConversationResponse struct {
	Stream chan StreamMessage
}

type StreamMessageType string

const (
	StreamMessageTypeAppend StreamMessageType = "append"
	StreamMessageTypeError  StreamMessageType = "error"
)

type StreamMessage struct {
	Delta          string            `json:"delta"`
	DeltaType      StreamMessageType `json:"delta_type"`
	MessageType    string            `json:"message_type"` // text
	ConversationID string            `json:"conversation_id"`
	MessageID      string            `json:"message_id"`
	ImagePath      string            `json:"image_path"`
	DeltaList      []*struct {
		Delta       string `json:"delta"`
		MessageType string `json:"message_type"` // text
	} `json:"delta_list"`
	Error error `json:"-"` // only StreamMessageTypeError
}

func (api *Api) ChatConversation(ctx context.Context, req *ChatConversationRequest) (*ChatConversationResponse, error) {
	urlStr := fmt.Sprintf("%s/api/chat/Chat", api.domain)
	resp, err := api.client.R().
		SetContext(ctx).
		SetBodyJsonMarshal(req).
		SetHeader("Accept", "*/*").
		DisableAutoReadResponse().
		Post(urlStr)
	if err != nil {
		return nil, err
	} else if resp.GetStatusCode() != http.StatusOK {
		return nil, fmt.Errorf("http error: code=%d, status=%s", resp.GetStatusCode(), resp.String())
	}

	reader := bufio.NewReader(resp.Body)
	msgChan := make(chan StreamMessage)

	go func() {
		defer func() {
			close(msgChan)
		}()

		for !resp.Close {
			line, err := reader.ReadString('\n')
			if err != nil && errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				msgChan <- StreamMessage{DeltaType: StreamMessageTypeError, Error: err}
				break
			}
			data := strings.TrimSpace(line)
			if data == "" || !strings.HasPrefix(data, "data:{") {
				continue
			}
			data = strings.TrimPrefix(data, "data:")

			var msg StreamMessage
			err = json.Unmarshal([]byte(data), &msg)
			if err != nil {
				msgChan <- StreamMessage{DeltaType: StreamMessageTypeError, Error: err}
				break
			} else if len(msg.DeltaList) == 0 {
				continue
			}

			msgChan <- msg
		}
	}()

	return &ChatConversationResponse{Stream: msgChan}, nil
}

type UserInfoResponse struct {
	Code  int64  `json:"code"`
	Email string `json:"email"`
	Msg   string `json:"msg"`
	Token string `json:"token"`
}

func (api *Api) UserInfo(ctx context.Context) (*UserInfoResponse, error) {
	urlStr := fmt.Sprintf("%s/api/user/info", api.domain)
	httpResp, err := api.client.R().
		SetContext(ctx).
		SetSuccessResult(UserInfoResponse{}).
		Post(urlStr)
	if err != nil {
		return nil, err
	} else if httpResp.GetStatusCode() != http.StatusOK {
		return nil, fmt.Errorf("http error: code=%d, status=%s", httpResp.GetStatusCode(), httpResp.String())
	}
	return httpResp.SuccessResult().(*UserInfoResponse), nil
}
