package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ognick/zabkiss/internal/domain"
	"github.com/ognick/zabkiss/pkg/logger"
	"github.com/ognick/zabkiss/pkg/visionllm"
)

type visionLLMGateway interface {
	ChatCompletion(ctx context.Context, messages []visionllm.Message) (string, error)
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
	llm      visionLLMGateway
	telegram telegramGateway
	repo     nutritionRepo
	log      logger.Logger
}

type AnalyzeFoodResult struct {
	Meal           domain.FoodLog
	Stats          domain.DailyStats
	Recommendation string
}

func NewNutritionService(llm visionLLMGateway, telegram telegramGateway, repo nutritionRepo, log logger.Logger) *NutritionService {
	return &NutritionService{llm: llm, telegram: telegram, repo: repo, log: log}
}

const foodAnalysisPrompt = `You are an expert nutritionist and food analyst.

Analyze the food image and estimate nutritional information.

Instructions:

1. Identify every visible food and beverage item.
2. Estimate the weight of each item in grams.
3. Estimate calories, protein, fat and carbohydrates for each item.
4. Calculate totals for the entire meal.
5. Estimate confidence for each item.
6. If confidence is low, explain uncertainty.
7. Use realistic values for cooked food.
8. Consider plate size, utensils, containers and visible proportions.
9. Do not invent precision when uncertain.
10. If multiple foods are mixed together, estimate their individual components.

Return ONLY valid JSON.

Schema:

{
  "meal_name": "",
  "overall_confidence": 0,
  "items": [
    {
      "name": "",
      "estimated_weight_g": 0,
      "calories": 0,
      "protein_g": 0,
      "fat_g": 0,
      "carbs_g": 0,
      "confidence": 0,
      "notes": ""
    }
  ],
  "totals": {
    "calories": 0,
    "protein_g": 0,
    "fat_g": 0,
    "carbs_g": 0
  },
  "analysis": {
    "portion_size": "",
    "notes": ""
  },
  "recommendation": ""
}

Rules:

* Return JSON only.
* No markdown.
* No explanations outside JSON.
* All numbers must be numeric.
* Confidence range: 0-100.
* If the food cannot be reliably identified, set confidence below 50.
* If the image quality is insufficient, explain why.
* recommendation: краткий диетологический совет по этому приёму пищи на русском языке, 1 предложение.`

type foodLLMResponse struct {
	MealName          string `json:"meal_name"`
	OverallConfidence int    `json:"overall_confidence"`
	Items             []struct {
		Name             string `json:"name"`
		EstimatedWeightG int    `json:"estimated_weight_g"`
		Calories         int    `json:"calories"`
		ProteinG         int    `json:"protein_g"`
		FatG             int    `json:"fat_g"`
		CarbsG           int    `json:"carbs_g"`
		Confidence       int    `json:"confidence"`
		Notes            string `json:"notes"`
	} `json:"items"`
	Totals struct {
		Calories int `json:"calories"`
		ProteinG int `json:"protein_g"`
		FatG     int `json:"fat_g"`
		CarbsG   int `json:"carbs_g"`
	} `json:"totals"`
	Analysis struct {
		PortionSize string `json:"portion_size"`
		Notes       string `json:"notes"`
	} `json:"analysis"`
	Recommendation string `json:"recommendation"`
}

func (s *NutritionService) AnalyzeFood(ctx context.Context, userID, fileID, caption string) (AnalyzeFoodResult, error) {
	s.log.Info("analyzing food", "user", userID, "file_id", fileID)

	imageBytes, err := s.telegram.DownloadFile(ctx, fileID)
	if err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("download photo: %w", err)
	}

	rawJSON, err := s.analyzeImage(ctx, imageBytes, caption)
	if err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("analyze food: %w", err)
	}

	analysis, err := parseFoodResponse(rawJSON)
	if err != nil {
		return AnalyzeFoodResult{}, fmt.Errorf("parse response: %w", err)
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

func (s *NutritionService) analyzeImage(ctx context.Context, imageBytes []byte, caption string) (string, error) {
	userText := "Analyze this food image."
	if caption != "" {
		userText += " User description: " + caption
	}

	base64Img := "data:image/jpeg;base64," + base64Encode(imageBytes)

	return s.llm.ChatCompletion(ctx, []visionllm.Message{
		{Role: "system", Content: []visionllm.ContentPart{{Type: "text", Text: foodAnalysisPrompt}}},
		{Role: "user", Content: []visionllm.ContentPart{
			{Type: "text", Text: userText},
			{Type: "image_url", ImageURL: base64Img},
		}},
	})
}

func parseFoodResponse(rawJSON string) (domain.FoodAnalysis, error) {
	rawJSON = strings.TrimSpace(rawJSON)
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var resp foodLLMResponse
	if err := json.Unmarshal([]byte(rawJSON), &resp); err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("parse food analysis: %w", err)
	}

	return domain.FoodAnalysis{
		Name:           resp.MealName,
		Calories:       resp.Totals.Calories,
		Protein:        resp.Totals.ProteinG,
		Fat:            resp.Totals.FatG,
		Carbs:          resp.Totals.CarbsG,
		Recommendation: resp.Recommendation,
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

func base64Encode(data []byte) string {
	const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf strings.Builder
	buf.Grow(((len(data) + 2) / 3) * 4)

	i := 0
	for i < len(data) {
		a := data[i]
		i++
		buf.WriteByte(encodeStd[a>>2])

		if i < len(data) {
			b := data[i]
			i++
			buf.WriteByte(encodeStd[((a&0x3)<<4)|(b>>4)])
			if i < len(data) {
				c := data[i]
				i++
				buf.WriteByte(encodeStd[((b&0xf)<<2)|(c>>6)])
				buf.WriteByte(encodeStd[c&0x3f])
			} else {
				buf.WriteByte(encodeStd[(b&0xf)<<2])
				buf.WriteByte('=')
			}
		} else {
			buf.WriteByte(encodeStd[(a&0x3)<<4])
			buf.WriteString("==")
		}
	}
	return buf.String()
}
