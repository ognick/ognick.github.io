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

const geminiBaseURL = "https://generativelanguage.googleapis.com/v1beta/models"

type Client interface {
	AnalyzeFood(ctx context.Context, imageBytes []byte, mimeType string, caption string) (domain.FoodAnalysis, error)
}

type geminiClient struct {
	apiKey string
	model  string
}

func NewClient(apiKey, model string) Client {
	return &geminiClient{apiKey: apiKey, model: model}
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text       string       `json:"text,omitempty"`
	InlineData *geminiImage `json:"inline_data,omitempty"`
}

type geminiImage struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
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

func (c *geminiClient) AnalyzeFood(ctx context.Context, imageBytes []byte, mimeType string, caption string) (domain.FoodAnalysis, error) {
	userText := "Analyze this food image."
	if caption != "" {
		userText += " User description: " + caption
	}

	req := geminiRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{Text: systemPrompt + "\n\n" + userText},
					{InlineData: &geminiImage{
						MimeType: mimeType,
						Data:     base64Encode(imageBytes),
					}},
				},
			},
		},
	}

	data, err := json.Marshal(req)
	if err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("marshal gemini request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("build gemini request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("gemini api call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return domain.FoodAnalysis{}, fmt.Errorf("gemini returned %d", resp.StatusCode)
	}

	var gemResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&gemResp); err != nil {
		return domain.FoodAnalysis{}, fmt.Errorf("decode gemini response: %w", err)
	}

	if len(gemResp.Candidates) == 0 || len(gemResp.Candidates[0].Content.Parts) == 0 {
		return domain.FoodAnalysis{}, fmt.Errorf("empty gemini response")
	}

	rawJSON := gemResp.Candidates[0].Content.Parts[0].Text
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
	// Use standard encoding without line breaks.
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
