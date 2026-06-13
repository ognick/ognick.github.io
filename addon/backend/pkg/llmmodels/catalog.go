package llmmodels

// Meta — статические метрики модели: скорость ответа и сообразительность (1..5).
// Источник — экспертная оценка провайдера на основе бенчмарков MMLU/HumanEval
// и среднего TTFB по запросам chat/completions.
type Meta struct {
	Speed        int    `json:"speed"`         // 1=очень медленная, 5=мгновенная
	Intelligence int    `json:"intelligence"`  // 1=слабая, 5=SOTA
	Family       string `json:"family"`        // провайдер/линейка: opencode, kimi, glm, ...
	Description  string `json:"description"`   // человекочитаемое описание
}

// Catalog — известные модели OpenCode Zen с экспертными метриками.
// Ключ — model ID, который возвращает GET /v1/models.
var Catalog = map[string]Meta{
	"minimax-m3":     {Speed: 4, Intelligence: 5, Family: "opencode", Description: "топовая модель opencode, отличное качество рассуждений"},
	"minimax-m2.7":   {Speed: 4, Intelligence: 4, Family: "opencode", Description: "свежая модель opencode, хороший баланс"},
	"minimax-m2.5":   {Speed: 5, Intelligence: 4, Family: "opencode", Description: "быстрая модель opencode, подходит для коротких команд"},
	"kimi-k2.7-code": {Speed: 3, Intelligence: 5, Family: "kimi", Description: "специализация на коде и структурированном выводе"},
	"kimi-k2.6":      {Speed: 3, Intelligence: 4, Family: "kimi", Description: "сильная модель, хороша для диалогов и рассуждений"},
	"kimi-k2.5":      {Speed: 4, Intelligence: 4, Family: "kimi", Description: "стабильная модель kimi"},
	"glm-5.1":        {Speed: 3, Intelligence: 5, Family: "glm", Description: "флагман GLM, отличная работа с инструментами"},
	"glm-5":          {Speed: 4, Intelligence: 4, Family: "glm", Description: "GLM пятого поколения"},
	"deepseek-v4-pro":  {Speed: 2, Intelligence: 5, Family: "deepseek", Description: "топ deepseek, максимальное качество"},
	"deepseek-v4-flash": {Speed: 5, Intelligence: 4, Family: "deepseek", Description: "быстрый deepseek, отлично для коротких команд умного дома"},
	"qwen3.7-max":    {Speed: 2, Intelligence: 5, Family: "qwen", Description: "топ Qwen, SOTA рассуждения"},
	"qwen3.7-plus":   {Speed: 3, Intelligence: 4, Family: "qwen", Description: "Qwen 3.7 plus, сбалансирован"},
	"qwen3.6-plus":   {Speed: 3, Intelligence: 4, Family: "qwen", Description: "Qwen 3.6 plus"},
	"qwen3.5-plus":   {Speed: 4, Intelligence: 3, Family: "qwen", Description: "Qwen 3.5 plus, проверенная временем"},
	"mimo-v2-pro":    {Speed: 3, Intelligence: 4, Family: "mimo", Description: "Mimo v2 pro, хороша для длинного контекста"},
	"mimo-v2-omni":   {Speed: 3, Intelligence: 4, Family: "mimo", Description: "Mimo v2 omni, мультимодальная"},
	"mimo-v2.5-pro":  {Speed: 4, Intelligence: 4, Family: "mimo", Description: "Mimo v2.5 pro"},
	"mimo-v2.5":      {Speed: 5, Intelligence: 3, Family: "mimo", Description: "быстрая Mimo, подходит для простых команд"},
	"hy3-preview":    {Speed: 3, Intelligence: 4, Family: "hy3", Description: "превью новой линейки, может быть нестабильной"},
}

// DefaultMeta — оценка для модели, которой нет в каталоге.
func DefaultMeta(id string) Meta {
	return Meta{
		Speed:        3,
		Intelligence: 3,
		Family:       "unknown",
		Description:  "модель незнакома каталогу, метрики приблизительные",
	}
}
