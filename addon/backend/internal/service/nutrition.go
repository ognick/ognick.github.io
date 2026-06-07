package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ognick/zabkiss/internal/domain"
	"github.com/ognick/zabkiss/pkg/logger"
)

type geminiGateway interface {
	AnalyzeFood(ctx context.Context, imageBytes []byte, mimeType string, caption string) (domain.FoodAnalysis, error)
}

type telegramGateway interface {
	DownloadFile(ctx context.Context, fileID string) ([]byte, error)
	SendMessage(ctx context.Context, chatID int, text string) error
}

type nutritionRepo interface {
	Save(ctx context.Context, log domain.FoodLog) error
	GetDailyStats(ctx context.Context, userID string, date time.Time) (domain.DailyStats, error)
}

type NutritionService struct {
	gemini   geminiGateway
	telegram telegramGateway
	repo     nutritionRepo
	log      logger.Logger
}

type AnalyzeFoodResult struct {
	Meal           domain.FoodLog
	Stats          domain.DailyStats
	Recommendation string
}

func NewNutritionService(gemini geminiGateway, telegram telegramGateway, repo nutritionRepo, log logger.Logger) *NutritionService {
	return &NutritionService{gemini: gemini, telegram: telegram, repo: repo, log: log}
}

func (s *NutritionService) AnalyzeFood(ctx context.Context, userID, fileID, caption string) (AnalyzeFoodResult, error) {
	s.log.Info("analyzing food", "user", userID, "file_id", fileID)

	imageBytes, err := s.telegram.DownloadFile(ctx, fileID)
	if err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("download photo: %w", err)
	}

	analysis, err := s.gemini.AnalyzeFood(ctx, imageBytes, "image/jpeg", caption)
	if err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("analyze food: %w", err)
	}

	s.log.Info("food analyzed",
		"name", analysis.Name,
		"calories", analysis.Calories,
		"protein", analysis.Protein,
		"fat", analysis.Fat,
		"carbs", analysis.Carbs,
	)

	now := time.Now()
	log := domain.FoodLog{
		UserID:    userID,
		Name:      analysis.Name,
		ImageRef:  fileID,
		Calories:  analysis.Calories,
		Protein:   analysis.Protein,
		Fat:       analysis.Fat,
		Carbs:     analysis.Carbs,
		Caption:   caption,
		CreatedAt: now,
	}

	if err := s.repo.Save(ctx, log); err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("save food log: %w", err)
	}

	stats, err := s.repo.GetDailyStats(ctx, userID, now)
	if err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("get daily stats: %w", err)
	}

	return AnalyzeFoodResult{
		Meal:           log,
		Stats:          stats,
		Recommendation: analysis.Recommendation,
	}, nil
}

func (s *NutritionService) GetDailyStats(ctx context.Context, userID string, date time.Time) (domain.DailyStats, error) {
	return s.repo.GetDailyStats(ctx, userID, date)
}

func FormatMealReply(meal domain.FoodLog, stats domain.DailyStats, recommendation string) string {
	reply := fmt.Sprintf("🍽 %s\nКБЖУ: %d ккал | Б: %dг | Ж: %dг | У: %dг",
		meal.Name, meal.Calories, meal.Protein, meal.Fat, meal.Carbs)

	reply += fmt.Sprintf("\n\n📊 За сегодня: %d ккал | Б: %dг | Ж: %dг | У: %dг",
		stats.TotalKcal, stats.Protein, stats.Fat, stats.Carbs)

	if recommendation != "" {
		reply += fmt.Sprintf("\n\n💡 %s", recommendation)
	}

	return reply
}
