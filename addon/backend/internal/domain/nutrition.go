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
