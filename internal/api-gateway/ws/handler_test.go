package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/LucasLCabral/payment-service/internal/api-gateway/ws"
	wsmocks "github.com/LucasLCabral/payment-service/internal/api-gateway/ws/mocks"
	"github.com/LucasLCabral/payment-service/pkg/logger"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"go.uber.org/mock/gomock"
)

type nopLog struct{}

func (nopLog) Debug(context.Context, string, ...any) {}
func (nopLog) Info(context.Context, string, ...any)  {}
func (nopLog) Warn(context.Context, string, ...any)  {}
func (nopLog) Error(context.Context, string, ...any) {}
func (nopLog) With(...any) logger.Logger              { return nopLog{} }

func TestHandler_ServeHTTP_preUpgrade(t *testing.T) {
	pid := uuid.MustParse("66666666-6666-4666-8666-666666666666")

	tests := []struct {
		name     string
		method   string
		query    string
		wantCode int
		wantSub  string
	}{
		{
			name:     "method not allowed",
			method:   http.MethodPost,
			query:    "payment_id=" + pid.String(),
			wantCode: http.StatusMethodNotAllowed,
			wantSub:  "method not allowed",
		},
		{
			name:     "invalid uuid",
			method:   http.MethodGet,
			query:    "payment_id=bad",
			wantCode: http.StatusBadRequest,
			wantSub:  "payment_id must be a valid UUID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			reg := wsmocks.NewMockConnRegistry(ctrl)

			h := &ws.Handler{Reg: reg, Log: nopLog{}}
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, "/ws?"+tt.query, nil)
			h.ServeHTTP(rr, req)

			if rr.Code != tt.wantCode {
				t.Fatalf("code = %d, want %d, body=%q", rr.Code, tt.wantCode, rr.Body.String())
			}
			if !strings.Contains(rr.Body.String(), tt.wantSub) {
				t.Fatalf("body %q should contain %q", rr.Body.String(), tt.wantSub)
			}
		})
	}
}

func TestHandler_ServeHTTP_websocketUpgrade(t *testing.T) {
	pid := uuid.MustParse("77777777-7777-4777-8777-777777777777")

	ctrl := gomock.NewController(t)
	reg := wsmocks.NewMockConnRegistry(ctrl)
	reg.EXPECT().Register(pid.String(), gomock.Any()).Times(1)
	reg.EXPECT().RemoveIf(pid.String(), gomock.Any()).Times(1)

	srv := httptest.NewServer(&ws.Handler{Reg: reg, Log: nopLog{}})
	defer srv.Close()

	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?payment_id=" + pid.String()
	conn, _, err := websocket.DefaultDialer.Dial(u, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.Close()
	time.Sleep(150 * time.Millisecond)
	ctrl.Finish()
}
