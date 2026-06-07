package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ognick/zabkiss/internal/domain"
)

type Client interface {
	AnalyzeFood(ctx context.Context, imageBytes []byte, mimeType string, caption string) (domain.FoodAnalysis, error)
}

type openAIClient struct {
	baseURL string
	apiKey  string
	model   string
}

func NewClient(baseURL, apiKey, model string) Client {
	return &openAIClient{baseURL: baseURL, apiKey: apiKey, model: model}
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type textContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type imageContent struct {
	Type     string    `json:"type"`
	ImageURL *imageURL `json:"image_url"`
}

type imageURL struct {
	URL string `json:"url"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type foodAnalysisResponse struct {
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

const systemPrompt = `You are an expert nutritionist and food analyst.

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

func (c *openAIClient) AnalyzeFood(ctx context.Context, imageBytes []byte, mimeType string, caption string) (domain.FoodAnalysis, error) {
	userText := "Analyze this food image."
	if caption != "" {
		userText += " User description: " + caption
	}

	base64Img := "data:" + mimeType + ";base64," + base64Encode(imageBytes)

	req := chatRequest{
		Model: c.model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: []interface{}{
				textContent{Type: "text", Text: userText},
				imageContent{Type: "image_url", ImageURL: &imageURL{URL: base64Img}},
			}},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(c.baseURL, "/") + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("api call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return domain.FoodAnalysis{}, fmt.Errorf("llm returned %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("decode response: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return domain.FoodAnalysis{}, fmt.Errorf("empty response")
	}

	rawJSON := chatResp.Choices[0].Message.Content
	rawJSON = strings.TrimSpace(rawJSON)
	rawJSON = strings.TrimPrefix(rawJSON, "```json")
	rawJSON = strings.TrimPrefix(rawJSON, "```")
	rawJSON = strings.TrimSuffix(rawJSON, "```")
	rawJSON = strings.TrimSpace(rawJSON)

	var analysis foodAnalysisResponse
	if err := json.Unmarshal([]byte(rawJSON), &analysis); err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("parse food analysis: %w", err)
	}

	return domain.FoodAnalysis{
		Name:           analysis.MealName,
		Calories:       analysis.Totals.Calories,
		Protein:        analysis.Totals.ProteinG,
		Fat:            analysis.Totals.FatG,
		Carbs:          analysis.Totals.CarbsG,
		Recommendation: analysis.Recommendation,
	}, nil
}

func base64Encode(data []byte) string {
	const encodeStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	var buf bytes.Buffer
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
