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

type Poller struct {
	svc          nutritionService
	client       tg.Client
	allowedUsers map[string]bool
	log          logger.Logger
	lastOffset   int
}

func NewPoller(svc nutritionService, client tg.Client, allowedUsers []string, log logger.Logger) *Poller {
	au := make(map[string]bool, len(allowedUsers))
	for _, id := range allowedUsers {
		au[id] = true
	}
	return &Poller{svc: svc, client: client, allowedUsers: au, log: log}
}

func (p *Poller) Run(ctx context.Context, probe func(error)) error {
	probe(nil)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		updates, err := p.client.GetUpdates(ctx, p.lastOffset+1)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			p.log.Error("getUpdates", "err", err)
			time.Sleep(5 * time.Second)
			continue
		}

		for _, update := range updates {
			if update.UpdateID > p.lastOffset {
				p.lastOffset = update.UpdateID
			}
			if update.Message == nil {
				continue
			}
			p.handleMessage(ctx, update.Message)
		}
	}
}

func (p *Poller) handleMessage(ctx context.Context, msg *tg.Message) {
	userID := fmt.Sprintf("%d", msg.From.ID)
	chatID := int(msg.Chat.ID)

	if !p.isAllowed(userID) {
		return
	}

	if len(msg.Photo) == 0 {
		if msg.Text != "" {
			p.log.Debug("no photo in message", "user", userID)
			if err := p.client.SendMessage(ctx, chatID, "Отправьте фото еды с описанием"); err != nil {
				p.log.Error("send no-photo reply", "err", err)
			}
		}
		return
	}

	bestPhoto := msg.Photo[len(msg.Photo)-1]

	go p.processPhoto(context.Background(), userID, chatID, bestPhoto.FileID, msg.Caption)
}

func (p *Poller) isAllowed(userID string) bool {
	return p.allowedUsers[userID]
}

func (p *Poller) processPhoto(ctx context.Context, userID string, chatID int, fileID, caption string) {
	ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	p.log.Info("processing photo", "user", userID, "chat", chatID, "file_id", fileID)

	result, err := p.svc.AnalyzeFood(ctx, userID, fileID, caption)
	if err != nil {
		p.log.Error("analyze food", "err", err)
		p.client.SendMessage(ctx, chatID, "Не удалось проанализировать фото. Попробуйте ещё раз.")
		return
	}

	reply := service.FormatMealReply(result.Meal, result.Stats, result.Recommendation)
	if err := p.client.SendMessage(ctx, chatID, reply); err != nil {
		p.log.Error("send telegram reply", "err", err)
	}
}
