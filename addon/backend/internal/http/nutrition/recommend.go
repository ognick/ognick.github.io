package nutrition

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/ognick/zabkiss/internal/domain"
)

type weeklyReportGenerator interface {
	ListByDate(ctx context.Context, userID string, date time.Time) ([]domain.FoodLog, error)
	MaxMealCreatedAt(ctx context.Context, userID string, start, end time.Time) (string, error)
}

func dailyRecommendation(targets, consumed domain.Macros) string {
	remaining := domain.Macros{
		Calories: targets.Calories - consumed.Calories,
		Protein:  targets.Protein - consumed.Protein,
		Fat:      targets.Fat - consumed.Fat,
		Carbs:    targets.Carbs - consumed.Carbs,
	}

	parts := []string{}

	if remaining.Calories > 0 {
		parts = append(parts, fmt.Sprintf("Осталось %d ккал до дневной нормы.", remaining.Calories))
	}
	if remaining.Protein > 0 {
		parts = append(parts, fmt.Sprintf("Добавьте %dг белка — творог, яйца или курицу.", remaining.Protein))
	}
	if remaining.Fat <= 0 && consumed.Fat > targets.Fat {
		parts = append(parts, "Норма жиров превышена, выбирайте менее жирные продукты.")
	}
	if remaining.Carbs > 50 {
		parts = append(parts, fmt.Sprintf("До нормы углеводов осталось %dг — добавьте крупы или цельнозерновой хлеб.", remaining.Carbs))
	}

	if consumed.Protein > 0 && consumed.Fat > 0 {
		ratio := float64(consumed.Protein) / float64(consumed.Fat)
		if ratio < 0.8 {
			parts = append(parts, "Баланс смещён в сторону жиров — увеличьте долю белка.")
		}
	}

	if len(parts) == 0 {
		return "Норма по всем показателям выполнена!"
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += " "
		}
		result += p
	}
	return result
}

func computeRequestHash(userID, period, targetsJSON, lastMealCreatedAt string) string {
	h := sha256.New()
	h.Write([]byte(userID))
	h.Write([]byte(period))
	h.Write([]byte(targetsJSON))
	h.Write([]byte(lastMealCreatedAt))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func formatPeriod(date time.Time) string {
	year, week := date.ISOWeek()
	return fmt.Sprintf("%d-W%02d", year, week)
}

func weekRange(date time.Time) (monday, sunday time.Time) {
	weekday := date.Weekday()
	if weekday == time.Sunday {
		weekday = 7
	}
	monday = date.AddDate(0, 0, -int(weekday-time.Monday))
	monday = time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, monday.Location())
	sunday = monday.AddDate(0, 0, 6)
	sunday = time.Date(sunday.Year(), sunday.Month(), sunday.Day(), 23, 59, 59, 0, sunday.Location())
	return
}

func generateWeeklyReport(ctx context.Context, repo weeklyReportGenerator, userID string, monday, sunday time.Time, targets domain.NutritionTargets) (domain.WeeklyReport, error) {
	period := formatPeriod(monday)

	var allMeals []domain.FoodLog
	var days []domain.DaySummary
	var totals domain.Macros
	dayCount := 0

	for d := monday; !d.After(sunday); d = d.AddDate(0, 0, 1) {
		meals, err := repo.ListByDate(ctx, userID, d)
		if err != nil {
			return domain.WeeklyReport{}, fmt.Errorf("list meals for %s: %w", d.Format("2006-01-02"), err)
		}
		if len(meals) > 0 {
			dayCount++
		}
		allMeals = append(allMeals, meals...)

		day := domain.DaySummary{Date: d.Format("2006-01-02")}
		for _, m := range meals {
			day.Calories += m.Calories
			day.Protein += m.Protein
			day.Fat += m.Fat
			day.Carbs += m.Carbs
		}
		days = append(days, day)
		totals.Calories += day.Calories
		totals.Protein += day.Protein
		totals.Fat += day.Fat
		totals.Carbs += day.Carbs
	}

	var avg domain.Macros
	if dayCount > 0 {
		avg.Calories = totals.Calories / dayCount
		avg.Protein = totals.Protein / dayCount
		avg.Fat = totals.Fat / dayCount
		avg.Carbs = totals.Carbs / dayCount
	}

	return domain.WeeklyReport{
		Period:         period,
		Days:           days,
		Totals:         totals,
		AverageDaily:   avg,
		Meals:          allMeals,
		Recommendation: "",
	}, nil
}

func weeklyLLMPrompt(report domain.WeeklyReport, targets domain.NutritionTargets) string {
	profile := ""
	if targets.HeightCm > 0 || targets.WeightKg > 0 {
		profile += fmt.Sprintf("\nПрофиль: рост %d см, вес %.1f кг", targets.HeightCm, targets.WeightKg)
	}
	if targets.Sex != "" {
		sexMap := map[string]string{"male": "мужчина", "female": "женщина"}
		if s, ok := sexMap[targets.Sex]; ok {
			profile += fmt.Sprintf(", пол: %s", s)
		}
	}
	if targets.BodyFatPct > 0 {
		profile += fmt.Sprintf(", %% жира: %.1f%%", targets.BodyFatPct)
	}

	return fmt.Sprintf(`Ты — эксперт-диетолог. Проанализируй недельное питание пользователя и дай краткие рекомендации на русском языке (3-5 предложений).

Цели КБЖУ в день: %d ккал, Б: %dг, Ж: %dг, У: %dг%s

Среднее потребление за неделю: %d ккал/день, Б: %dг, Ж: %dг, У: %dг
Всего за неделю: %d ккал

Дневная разбивка (ккал):
%s

Напиши 3-5 предложений с анализом и рекомендациями. Учитывай баланс БЖУ, общую калорийность, динамику по дням.`,
		targets.Calories, targets.Protein, targets.Fat, targets.Carbs, profile,
		report.AverageDaily.Calories, report.AverageDaily.Protein, report.AverageDaily.Fat, report.AverageDaily.Carbs,
		report.Totals.Calories,
		dailyBreakdown(report.Days),
	)
}

func sanitizeForPrompt(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "\x60", `\`+"\x60")
	return s
}

func dailyBreakdown(days []domain.DaySummary) string {
	var s string
	for _, d := range days {
		s += fmt.Sprintf("  %s: %d ккал (Б:%d Ж:%d У:%d)\n", sanitizeForPrompt(d.Date), d.Calories, d.Protein, d.Fat, d.Carbs)
	}
	return s
}
