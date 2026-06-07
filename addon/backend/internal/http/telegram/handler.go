package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ognick/zabkiss/internal/service"
	"github.com/ognick/zabkiss/pkg/logger"
)

type nutritionService interface {
	AnalyzeFood(ctx context.Context, userID, fileID, caption string) (service.AnalyzeFoodResult, error)
}

type telegramSender interface {
	SendMessage(ctx context.Context, chatID int, text string) error
}

type Handler struct {
	svc          nutritionService
	sender       telegramSender
	allowedUsers map[string]bool
	log          logger.Logger
}

func NewHandler(svc nutritionService, sender telegramSender, allowedUsers []string, log logger.Logger) *Handler {
	au := make(map[string]bool, len(allowedUsers))
	for _, id := range allowedUsers {
		au[id] = true
	}
	return &Handler{svc: svc, sender: sender, allowedUsers: au, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Post("/telegram/webhook", h.webhook)
}

type telegramUpdate struct {
	UpdateID int              `json:"update_id"`
	Message  *telegramMessage `json:"message"`
}

type telegramMessage struct {
	MessageID int          `json:"message_id"`
	From      telegramUser `json:"from"`
	Chat      telegramChat `json:"chat"`
	Photo     []photoSize  `json:"photo"`
	Caption   string       `json:"caption"`
	Text      string       `json:"text"`
}

type telegramUser struct {
	ID int64 `json:"id"`
}

type telegramChat struct {
	ID int64 `json:"id"`
}

type photoSize struct {
	FileID string `json:"file_id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error("read telegram webhook body", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var update telegramUpdate
	if err := json.Unmarshal(body, &update); err != nil {
		h.log.Error("parse telegram update", "err", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if update.Message == nil || update.Message.From.ID == 0 || update.Message.Chat.ID == 0 {
		w.WriteHeader(http.StatusOK)
		return
	}

	msg := update.Message
	userID := fmt.Sprintf("%d", msg.From.ID)
	chatID := int(msg.Chat.ID)

	if !h.isAllowed(userID) {
		w.WriteHeader(http.StatusOK)
		return
	}

	if len(msg.Photo) == 0 {
		h.log.Debug("no photo in message", "user", userID)
		if msg.Text != "" {
			if err := h.sender.SendMessage(r.Context(), chatID, "Отправьте фото еды с описанием"); err != nil {
				h.log.Error("send no-photo reply", "err", err)
			}
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	bestPhoto := msg.Photo[len(msg.Photo)-1]
	caption := msg.Caption

	go h.processPhoto(context.Background(), userID, chatID, bestPhoto.FileID, caption)

	w.WriteHeader(http.StatusOK)
}

func (h *Handler) isAllowed(userID string) bool {
	return h.allowedUsers[userID]
}

func (h *Handler) processPhoto(ctx context.Context, userID string, chatID int, fileID, caption string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	h.log.Info("processing photo", "user", userID, "chat", chatID, "file_id", fileID)

	result, err := h.svc.AnalyzeFood(ctx, userID, fileID, caption)
	if err != nil {
		h.log.Error("analyze food", "err", err)
		h.sender.SendMessage(ctx, chatID, "Не удалось проанализировать фото. Попробуйте ещё раз.")
		return
	}

	reply := service.FormatMealReply(result.Meal, result.Stats, result.Recommendation)
	if err := h.sender.SendMessage(ctx, chatID, reply); err != nil {
		h.log.Error("send telegram reply", "err", err)
	}
}
