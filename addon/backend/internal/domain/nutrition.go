package domain

import "time"

type FoodLog struct {
	ID        int64     `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	ImageRef  string    `json:"image_ref"`
	Calories  int       `json:"calories"`
	Protein   int       `json:"protein"`
	Fat       int       `json:"fat"`
	Carbs     int       `json:"carbs"`
	Caption   string    `json:"caption"`
	CreatedAt time.Time `json:"created_at"`
}

type DailyStats struct {
	Date      string    `json:"date"`
	TotalKcal int       `json:"total_kcal"`
	Protein   int       `json:"protein"`
	Fat       int       `json:"fat"`
	Carbs     int       `json:"carbs"`
	Meals     []FoodLog `json:"meals"`
}

type FoodAnalysis struct {
	Name           string `json:"meal_name"`
	Calories       int    `json:"calories"`
	Protein        int    `json:"protein"`
	Fat            int    `json:"fat"`
	Carbs          int    `json:"carbs"`
	Recommendation string `json:"recommendation"`
}

type NutritionTargets struct {
	UserID     string  `json:"user_id"`
	Calories   int     `json:"calories"`
	Protein    int     `json:"protein"`
	Fat        int     `json:"fat"`
	Carbs      int     `json:"carbs"`
	HeightCm   int     `json:"height_cm"`
	WeightKg   float64 `json:"weight_kg"`
	Sex        string  `json:"sex"`
	BodyFatPct float64 `json:"body_fat_pct"`
	APIToken   string  `json:"api_token,omitempty"`
}

type Macros struct {
	Calories int `json:"calories"`
	Protein  int `json:"protein"`
	Fat      int `json:"fat"`
	Carbs    int `json:"carbs"`
}

type DailyReport struct {
	Date           string    `json:"date"`
	Consumed       Macros    `json:"consumed"`
	Remaining      Macros    `json:"remaining"`
	Targets        Macros    `json:"targets"`
	Meals          []FoodLog `json:"meals"`
	Recommendation string    `json:"recommendation"`
}

type WeeklyReport struct {
	Period         string       `json:"period"`
	Days           []DaySummary `json:"days"`
	Totals         Macros       `json:"totals"`
	AverageDaily   Macros       `json:"average_daily"`
	Meals          []FoodLog    `json:"meals"`
	Recommendation string       `json:"recommendation"`
}

type DaySummary struct {
	Date     string `json:"date"`
	Calories int    `json:"calories"`
	Protein  int    `json:"protein"`
	Fat      int    `json:"fat"`
	Carbs    int    `json:"carbs"`
}
