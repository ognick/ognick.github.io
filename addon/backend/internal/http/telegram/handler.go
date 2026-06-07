package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/ognick/zabkiss/internal/service"
	"github.com/ognick/zabkiss/pkg/logger"
	tg "github.com/ognick/zabkiss/pkg/telegram"
)

type nutritionService interface {
	AnalyzeFood(ctx context.Context, userID, fileID, caption string) (service.AnalyzeFoodResult, error)
}

type Handler struct {
	svc          nutritionService
	client       tg.Client
	allowedUsers map[string]bool
	log          logger.Logger
	lastOffset   int
}

func NewHandler(svc nutritionService, client tg.Client, allowedUsers []string, log logger.Logger) *Handler {
	au := make(map[string]bool, len(allowedUsers))
	for _, id := range allowedUsers {
		au[id] = true
	}
	return &Handler{svc: svc, client: client, allowedUsers: au, log: log}
}

func (h *Handler) Run(ctx context.Context, probe func(error)) error {
	probe(nil)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := h.client.GetUpdates(ctx, h.lastOffset+1)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			h.log.Error("getUpdates", "err", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID > h.lastOffset {
				h.lastOffset = update.UpdateID
			}
			if update.Message == nil {
				continue
			}
			h.handleMessage(ctx, update.Message)
		}
	}
}

func (h *Handler) handleMessage(ctx context.Context, msg *tg.Message) {
	userID := fmt.Sprintf("%d", msg.From.ID)
	chatID := int(msg.Chat.ID)

	if !h.isAllowed(userID) {
		return
	}

	if len(msg.Photo) == 0 {
		if msg.Text != "" {
			h.log.Debug("no photo in message", "user", userID)
			if err := h.client.SendMessage(ctx, chatID, "Отправьте фото еды с описанием"); err != nil {
				h.log.Error("send no-photo reply", "err", err)
			}
		}
		return
	}

	bestPhoto := msg.Photo[len(msg.Photo)-1]

	go h.processPhoto(context.Background(), userID, chatID, bestPhoto.FileID, msg.Caption)
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
		h.client.SendMessage(ctx, chatID, "Не удалось проанализировать фото. Попробуйте ещё раз.")
		return
	}

	reply := service.FormatMealReply(result.Meal, result.Stats, result.Recommendation)
	if err := h.client.SendMessage(ctx, chatID, reply); err != nil {
		h.log.Error("send telegram reply", "err", err)
	}
}
