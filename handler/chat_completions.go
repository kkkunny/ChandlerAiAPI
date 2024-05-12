package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	stlbasic "github.com/kkkunny/stl/basic"
	stlslices "github.com/kkkunny/stl/container/slices"
	"github.com/sashabaranov/go-openai"
	"golang.org/x/sync/errgroup"

	"github.com/kkkunny/ChandlerAiAPI/internal/api"
	"github.com/kkkunny/ChandlerAiAPI/internal/config"
	"github.com/kkkunny/ChandlerAiAPI/internal/consts"
)

func ChatCompletions(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	cli := api.NewAPI(consts.ChandlerAiDomain, token, nil)

	var req openai.ChatCompletionRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		config.Logger.Error(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	err = json.Unmarshal(body, &req)
	if err != nil {
		config.Logger.Error(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	eg, _ := errgroup.WithContext(r.Context())

	var conv *api.SimpleConversationInfo
	var lastMsgID string
	eg.Go(func() error {
		listConvResp, err := cli.ListConversations(r.Context(), &api.ListConversationsRequest{
			ModelNames: []string{req.Model},
			PageNum:    1,
			PageSize:   10,
		})
		if err != nil {
			return err
		} else if stlslices.Empty(listConvResp.Data) {
			return nil
		}
		conv = stlslices.Random(listConvResp.Data)

		convInfoResp, err := cli.ConversationInfo(r.Context(), &api.ConversationInfoRequest{
			ConversationID: conv.ConversationID,
			IsV2:           true,
			WebUrl:         consts.ChandlerAiDomain,
		})
		if err != nil {
			return err
		}
		lastMsgID = stlslices.Last(convInfoResp.Data).MessageID
		return nil
	})

	var email string
	eg.Go(func() error {
		resp, err := cli.UserInfo(r.Context())
		if err != nil {
			return err
		}
		email = resp.Email
		return nil
	})

	err = eg.Wait()
	if err != nil {
		config.Logger.Error(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	msgStrList := make([]string, len(req.Messages)+1)
	msgStrList[0] = "Forget previous messages and focus on the current message!\n"
	for i, msg := range req.Messages {
		msgStrList[i+1] = fmt.Sprintf("%s: %s", msg.Role, msg.Content)
	}
	prompt := fmt.Sprintf("%s\nassistant: ", strings.Join(msgStrList, ""))

	chatResp, err := cli.ChatConversation(r.Context(), &api.ChatConversationRequest{
		AppName: stlbasic.TernaryAction(conv == nil, func() string {
			return ""
		}, func() string {
			return conv.AppName
		}),
		ConversationID: stlbasic.TernaryAction(conv == nil, func() string {
			return ""
		}, func() string {
			return conv.ConversationID
		}),
		GlobalTimeout:   100,
		MaxRetries:      1,
		ModelName:       req.Model,
		ParentMessageID: lastMsgID,
		Prompt:          prompt,
		RequestTimeout:  30,
		Timestamp:       time.Now().UnixMilli(),
		UID:             email,
		WebUrl:          consts.ChandlerAiDomain,
	})
	if err != nil {
		config.Logger.Error(err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	if !req.Stream {
		var reply strings.Builder
		var msgID string
		var tokenCount uint64
		for msg := range chatResp.Stream {
			switch msg.DeltaType {
			case api.StreamMessageTypeError:
				config.Logger.Error(err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			case api.StreamMessageTypeAppend:
				tokenCount++
				reply.WriteString(msg.Delta)
				msgID = msg.MessageID
			default:
				config.Logger.Warnf("unknown stream msg type `%s`", msg.DeltaType)
			}
		}

		data, err := json.Marshal(&openai.ChatCompletionResponse{
			ID:      msgID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []openai.ChatCompletionChoice{
				{
					Index: 0,
					Message: openai.ChatCompletionMessage{
						Role:    "assistant",
						Content: reply.String(),
					},
					FinishReason: "stop",
				},
			},
			Usage: openai.Usage{
				PromptTokens:     0,
				CompletionTokens: int(tokenCount),
				TotalTokens:      int(tokenCount),
			},
		})
		if err != nil {
			config.Logger.Error(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = fmt.Fprint(w, string(data))
		if err != nil {
			config.Logger.Error(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	} else {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Transfer-Encoding", "chunked")

		flusher := w.(http.Flusher)
		var msgID string

		for msg := range chatResp.Stream {
			switch msg.DeltaType {
			case api.StreamMessageTypeError:
				config.Logger.Error(err)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			case api.StreamMessageTypeAppend:
				msgID = msg.MessageID
				data, err := json.Marshal(&openai.ChatCompletionStreamResponse{
					ID:      msgID,
					Object:  "chat.completion",
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []openai.ChatCompletionStreamChoice{
						{
							Index: 0,
							Delta: openai.ChatCompletionStreamChoiceDelta{
								Role:    "assistant",
								Content: msg.Delta,
							},
						},
					},
				})
				if err != nil {
					config.Logger.Error(err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				_, err = fmt.Fprint(w, "data: "+string(data)+"\n")
				if err != nil {
					config.Logger.Error(err)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
					return
				}
				flusher.Flush()
			default:
				config.Logger.Warnf("unknown stream msg type `%s`", msg.DeltaType)
			}
		}

		data, err := json.Marshal(&openai.ChatCompletionStreamResponse{
			ID:      msgID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []openai.ChatCompletionStreamChoice{
				{
					Index:        0,
					FinishReason: "stop",
				},
			},
		})
		if err != nil {
			config.Logger.Error(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		_, err = fmt.Fprint(w, "data: "+string(data)+"\n\n")
		if err != nil {
			config.Logger.Error(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		flusher.Flush()
		_, err = fmt.Fprint(w, "data: [DONE]\n\n")
		if err != nil {
			config.Logger.Error(err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		flusher.Flush()
	}
}
