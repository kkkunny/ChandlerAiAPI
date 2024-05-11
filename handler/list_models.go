package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/sashabaranov/go-openai"

	"github.com/kkkunny/ChandlerAiAPI/internal/config"
)

func ListModels(w http.ResponseWriter, _ *http.Request) {
	ownerBy := "ChandlerAi"
	data, err := json.Marshal(&openai.ModelsList{
		Models: []openai.Model{
			{
				CreatedAt: 1692901427,
				ID:        "gpt-3.5",
				Object:    "model",
				OwnedBy:   ownerBy,
			},
			{
				CreatedAt: 1692901427,
				ID:        "llama3-70b",
				Object:    "model",
				OwnedBy:   ownerBy,
			},
			{
				CreatedAt: 1692901427,
				ID:        "llama3-8b",
				Object:    "model",
				OwnedBy:   ownerBy,
			},
			{
				CreatedAt: 1692901427,
				ID:        "grok",
				Object:    "model",
				OwnedBy:   ownerBy,
			},
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
}
