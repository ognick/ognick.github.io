package alice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ognick/zabkiss/internal/domain"
	"github.com/ognick/zabkiss/internal/service"
	"github.com/ognick/zabkiss/pkg/logger"
)

var (
	errReadBody  = errors.New("не удалось прочитать запрос")
	errParseBody = errors.New("не удалось разобрать запрос")
	errAuth      = errors.New("пожалуйста авторизуйтесь для продолжения")
	errInternal  = errors.New("internal error")
)

// maxResponseTime — резерв времени на отправку ответа Алисе
// (JSON-кодирование + сетевая задержка).
const maxResponseTime = 300 * time.Millisecond

// maxBodyBytes — предел размера тела запроса от Алисы. Реальные webhook'ы
// Алисы не превышают пары килобайт; 64 KB — щедрый потолок, защищающий от
// DoS через огромные JSON-тела.
const maxBodyBytes int64 = 64 * 1024

// maxCommandBytes — предел длины пользовательской команды, передаваемой в LLM.
// Голосовые команды Алисы обычно <1 KB; 4 KB — щедрый потолок, защищающий
// от cost-amplification атак через огромные тексты.
const maxCommandBytes = 4 * 1024

type commandService interface {
	Process(ctx context.Context, sessionID, userID, command string) (domain.CommandResult, error)
	PopInbox(sessionID string) string
	StoreInbox(sessionID, reply string)
}

type userResolver interface {
	ResolveUser(ctx context.Context, token string) (domain.User, error)
}

type Handler struct {
	svc  commandService
	auth userResolver
	log  logger.Logger
}

func New(svc commandService, auth userResolver, log logger.Logger) *Handler {
	return &Handler{svc: svc, auth: auth, log: log}
}

func (h *Handler) Register(r chi.Router) {
	r.Route("/alice", func(r chi.Router) {
		r.Post("/webhook", h.webhook)
	})
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, errReadBody)
		return
	}

	var req aliceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.writeError(w, errParseBody)
		return
	}

	// Truncate oversized commands to prevent LLM cost amplification.
	if len(req.Request.Command) > maxCommandBytes {
		h.log.Warn("command truncated", "session", req.Session.SessionID, "size", len(req.Request.Command))
		req.Request.Command = req.Request.Command[:maxCommandBytes]
	}

	if req.Request.OriginalUtterance == "ping" {
		h.log.Info("health check", "session_id", req.Session.SessionID)
		h.write(w, aliceResponse{
			Version:  version,
			Response: responseBody{Text: "ok"},
		})
		return
	}

	h.log.Info("webhook request", "session", req.Session.SessionID, "utterance", req.Request.OriginalUtterance)

	// Auth не зависит от Alice-таймаута — используем контекст HTTP-запроса.
	user, err := h.resolveAuth(r.Context(), req)
	if err != nil {
		h.log.Warn("auth failed", "session", req.Session.SessionID, "err", err)
		if errors.Is(err, errForbidden) {
			msg := fmt.Sprintf("%s, %s", user.Name, errForbidden.Error())
			h.write(w, aliceResponse{
				Version:  version,
				Response: responseBody{Text: msg, TTS: msg},
			})
			return
		}
		if !errors.Is(err, errAuth) {
			h.log.Error(err.Error())
		}
		h.write(w, aliceResponse{
			Version: version,
			Response: responseBody{
				Text:       errAuth.Error(),
				Directives: &directives{StartAccountLinking: &struct{}{}},
			},
		})
		return
	}

	h.log.Info("auth ok", "session", req.Session.SessionID, "user", user.Name, "email", user.Email)

	aliceTimeout := h.getAliceTimeout(r)

	resultCh := make(chan domain.CommandResult, 1)
	errCh := make(chan error, 1)
	go func() {
		// Recover внутри горутины: defer/recover в middleware ловить
		// только панику текущего стека, а не панику в спавн-горутине.
		defer func() {
			if rec := recover(); rec != nil {
				h.log.Error("panic in process goroutine", "err", rec, "session", req.Session.SessionID)
				errCh <- errInternal
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), service.ProcessTimeout)
		defer cancel()
		result, err := h.svc.Process(ctx, req.Session.SessionID, user.ID, req.Request.Command)
		if err != nil {
			errCh <- err
		} else {
			resultCh <- result
		}
	}()

	var timeoutCh <-chan time.Time
	if aliceTimeout > 0 {
		timeoutCh = time.After(aliceTimeout)
	}

	select {
	case result := <-resultCh:
		pending := h.svc.PopInbox(req.Session.SessionID)
		reply := result.Reply
		if pending != "" {
			reply = service.JoinReplies(pending, reply)
		}
		if !result.EndSession {
			reply = service.WithOpenQuestion(reply)
		}
		h.write(w, aliceResponse{
			Version:  version,
			Response: responseBody{Text: reply, TTS: reply, EndSession: result.EndSession},
		})

	case err := <-errCh:
		h.log.Error("process command", "err", err)
		if errors.Is(err, errInternal) {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		h.writeError(w, fmt.Errorf("%s, произошла ошибка при обработке команды", user.Name))

	case <-timeoutCh:
		h.log.Warn("alice deadline exceeded, deferring result", "session", req.Session.SessionID)
		go func() {
			defer func() {
				if rec := recover(); rec != nil {
					h.log.Error("panic in deferred goroutine", "err", rec, "session", req.Session.SessionID)
				}
			}()
			select {
			case result := <-resultCh:
				h.log.Info("deferred result ready, storing in inbox", "session", req.Session.SessionID)
				h.svc.StoreInbox(req.Session.SessionID, result.Reply)
			case err := <-errCh:
				h.log.Error("deferred processing failed", "session", req.Session.SessionID, "err", err)
			case <-time.After(service.ProcessTimeout):
				h.log.Warn("deferred processing timed out", "session", req.Session.SessionID)
			}
		}()
		pending := h.svc.PopInbox(req.Session.SessionID)
		msg := "Обрабатываю запрос, спроси чуть позже"
		if pending != "" {
			msg = service.JoinReplies(pending, msg)
		}
		h.write(w, aliceResponse{
			Version:  version,
			Response: responseBody{Text: msg, TTS: msg},
		})
	}
}

// getAliceTimeout парсит Request-Timeout (микросекунды) и возвращает duration
// с резервом maxResponseTime на отправку ответа.
func (h *Handler) getAliceTimeout(r *http.Request) time.Duration {
	v := r.Header.Get("Request-Timeout")
	if v == "" {
		return 0
	}
	us, err := strconv.ParseInt(v, 10, 64)
	if err != nil || us <= 0 {
		return 0
	}
	timeout := time.Duration(us) * time.Microsecond
	if timeout <= maxResponseTime {
		return 0
	}
	return timeout - maxResponseTime
}

func (h *Handler) resolveAuth(ctx context.Context, req aliceRequest) (domain.User, error) {
	token := req.Session.User.AccessToken
	yandexID := req.Session.User.UserID
	if token == "" || yandexID == "" {
		return domain.User{}, errAuth
	}
	return h.auth.ResolveUser(ctx, token)
}

func (h *Handler) write(w http.ResponseWriter, resp aliceResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.log.Warn("alice response write failed (connection likely closed)", "err", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, err error) {
	h.log.Error(err.Error())
	h.write(w, aliceResponse{
		Version:  version,
		Response: responseBody{Text: err.Error(), TTS: err.Error(), EndSession: true},
	})
}
