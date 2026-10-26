package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/relaykit/dto"

	"phanthycode2api/internal/phanthy"
	"phanthycode2api/internal/pool"
	"phanthycode2api/internal/protocol"
)

// errEmptyStream 表示上游没有返回任何有效内容。
// 旧实现会把它伪造成一次成功的空响应，这里显式视为失败。
var errEmptyStream = errors.New("上游返回空流")

// handleChat 处理 OpenAI 兼容的 chat completions。
func (g *Gateway) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method_not_allowed", "只支持 POST")
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "body_too_large", "请求体读取失败或超过 8 MiB")
		return
	}

	var req protocol.ChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_json", "请求体不是合法 JSON: "+err.Error())
		return
	}

	upstreamBody, err := protocol.BuildAnthropicRequest(r.Context(), &req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request_error", "invalid_request", err.Error())
		return
	}
	estPrompt := protocol.EstimateRequestTokens(req.Model, &req)

	tried := make(map[string]bool, g.MaxRotate)
	var lastErr error

	for attempt := 0; attempt < g.MaxRotate; attempt++ {
		now := g.now()
		cred := g.pool.PickExcluding(now, tried)
		if cred == nil {
			break
		}
		tried[cred.UID] = true

		if err := g.upstream.RefreshCredential(r.Context(), cred, g.RefreshSkew); err != nil {
			g.logger.Printf("账号 %s token 刷新失败: %v", cred.UID, err)
			g.noteFailure(cred.UID, err)
			lastErr = err
			continue
		}

		snap := cred.Snapshot()
		g.pool.MarkUsed(snap.UID, now)
		g.logger.Printf("请求 %s 使用账号 %s（第 %d 次尝试）", req.Model, snap.UID, attempt+1)

		resp, err := g.upstream.Messages(r.Context(), phanthy.Auth{
			AccessToken: snap.AccessToken,
			APIKey:      snap.APIKey,
			UID:         snap.UID,
		}, upstreamBody)
		if err != nil {
			var ue *phanthy.Error
			if errors.As(err, &ue) && ue.Kind == phanthy.KindClient {
				// 请求本身有问题，换账号也无济于事。
				writeError(w, http.StatusBadRequest, "invalid_request_error", "upstream_client_error", ue.Message)
				return
			}
			g.logger.Printf("账号 %s 上游调用失败: %v", snap.UID, err)
			g.noteFailure(snap.UID, err)
			lastErr = err
			continue
		}

		collector := protocol.NewCollector(req.Model, estPrompt)
		done, relayErr := g.relay(w, r, resp, collector, req.Stream)
		if done {
			g.pool.NoteSuccess(snap.UID)
			return
		}
		g.logger.Printf("账号 %s 响应失败: %v", snap.UID, relayErr)
		g.noteFailure(snap.UID, relayErr)
		lastErr = relayErr
	}

	message := "all accounts unavailable (no healthy account in pool)"
	if lastErr != nil {
		message = fmt.Sprintf("all accounts unavailable (%v)", lastErr)
	}
	writeError(w, http.StatusServiceUnavailable, "api_error", "no_healthy_account", message)
}

// relay 消费上游响应。
// 返回 done=true 表示已完整响应客户端；done=false 表示尚未写出任何字节，可以换号重试。
func (g *Gateway) relay(w http.ResponseWriter, r *http.Request, resp *http.Response, collector *protocol.Collector, stream bool) (bool, error) {
	defer resp.Body.Close()

	if !stream {
		if err := collector.Run(resp.Body, nil); err != nil {
			return false, err
		}
		if !collector.HasOutput() {
			return false, errEmptyStream
		}
		writeJSON(w, http.StatusOK, collector.Completion())
		return true, nil
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "api_error", "streaming_unsupported", "当前服务器不支持流式响应")
		return true, nil
	}

	committed := false
	commit := func() {
		h := w.Header()
		h.Set("Content-Type", "text/event-stream; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		h.Set("Connection", "keep-alive")
		h.Set("X-Accel-Buffering", "no")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()
	}

	onChunk := func(chunk *dto.ChatCompletionsStreamResponse) error {
		frame, err := collector.EncodeChunk(chunk)
		if err != nil {
			return err
		}
		if !committed {
			commit()
			committed = true
		}
		if _, err := w.Write(frame); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	if err := collector.Run(resp.Body, onChunk); err != nil {
		if !committed {
			// 还没有写出任何内容，可以交给上层换号重试。
			return false, err
		}
		g.logger.Printf("流式传输中断: %v", err)
		writeSSEError(w, flusher, err)
		return true, nil
	}

	if !committed {
		if !collector.HasOutput() {
			return false, errEmptyStream
		}
		commit()
		committed = true
	}

	if final := collector.FinalChunk(); final != nil {
		frame, err := collector.EncodeChunk(final)
		if err != nil {
			return true, nil
		}
		if _, err := w.Write(frame); err != nil {
			return true, nil
		}
	}
	if _, err := w.Write([]byte(protocol.DoneFrame)); err != nil {
		return true, nil
	}
	flusher.Flush()
	return true, nil
}

// writeSSEError 在响应头已写出后，用一条 SSE 错误帧终止流。
func writeSSEError(w http.ResponseWriter, flusher http.Flusher, err error) {
	payload, _ := json.Marshal(errorEnvelope{Error: errorDetail{
		Message: err.Error(),
		Type:    "api_error",
		Code:    "upstream_stream_error",
	}})
	frame := append([]byte("data: "), payload...)
	frame = append(frame, '\n', '\n')
	_, _ = w.Write(frame)
	_, _ = w.Write([]byte(protocol.DoneFrame))
	flusher.Flush()
}

// noteFailure 按错误分类驱动账号池状态机（FR-08…FR-10、FR-16）。
func (g *Gateway) noteFailure(uid string, err error) {
	now := g.now()
	var ue *phanthy.Error
	if errors.As(err, &ue) {
		switch ue.Kind {
		case phanthy.KindHardCredit:
			g.pool.Cooldown(uid, pool.CooldownHard, g.cfg.HardCreditDur, reasonOf(ue), now)
			g.pool.SetBalance(uid, int64Ptr(0))
			return
		case phanthy.KindSessionDead:
			g.pool.Disable(uid, reasonOf(ue))
			return
		case phanthy.KindSoftRate:
			g.pool.Cooldown(uid, pool.CooldownSoft, g.cfg.SoftRateDur, reasonOf(ue), now)
			return
		case phanthy.KindNotFound:
			g.pool.Cooldown(uid, pool.CooldownSoft, g.cfg.SoftRateDur, "not_found", now)
			return
		}
	}
	if g.pool.NoteError(uid, g.cfg.Cooldown.ErrThreshold, g.cfg.ErrCooldownDur, now) {
		g.logger.Printf("账号 %s 连续错误达到阈值，冷却 %s", uid, g.cfg.ErrCooldownDur)
	}
}

func reasonOf(ue *phanthy.Error) string {
	if ue == nil {
		return ""
	}
	if ue.Message != "" {
		return ue.Message
	}
	return ue.Kind.String()
}

func int64Ptr(v int64) *int64 { return &v }
