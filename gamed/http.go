package gamed

import (
	"bytes"
	"cmp"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/game/game/apis/basepb"
	"github.com/goccy/go-json"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"

	"github.com/rs/cors"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/spf13/viper"
)

type responseWrapper struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func newResponseWrapper(w http.ResponseWriter) *responseWrapper {
	return &responseWrapper{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
		body:           bytes.NewBuffer(nil),
	}
}

func (w *responseWrapper) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

func (w *responseWrapper) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *responseWrapper) finalize() {
	w.ResponseWriter.Header().Del("Vary")
	w.ResponseWriter.Header().Del("grpc-metadata-content-type")
	w.ResponseWriter.Header().Del("Content-Length")
	w.ResponseWriter.WriteHeader(http.StatusOK)

	response := make(map[string]any, 3)
	if w.statusCode >= http.StatusBadRequest {
		var errBody struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(w.body.Bytes(), &errBody); err == nil {
			response = map[string]any{
				"code":    errBody.Code,
				"message": errBody.Message,
			}
		} else {
			log.Error().Bytes("body", w.body.Bytes()).Msg("parse error response body failed")
			response = map[string]any{
				"code":    w.statusCode,
				"message": w.body.String(),
			}
		}
	} else {
		response["code"] = 0
		response["message"] = "success"
		if w.body.Len() > 0 {
			response["data"] = json.RawMessage(w.body.Bytes())
		}
	}
	json.NewEncoder(w.ResponseWriter).Encode(response)
}

// gatewayHandler 对请求响应格式进行包装
func gatewayHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))

		wrapper := newResponseWrapper(w)
		h = cors.AllowAll().Handler(h)
		h.ServeHTTP(wrapper, r.WithContext(ctx))
		wrapper.finalize()
	})
}

// initHttpHandler 初始化HTTP处理器，聚合静态文件和API
func initHttpHandler(gwmux *runtime.ServeMux) http.Handler {
	rootMux := http.NewServeMux()

	// 1. 静态文件服务
	staticPrefix := viper.GetString("static.prefix")
	if staticPrefix != "" {
		staticDir := cmp.Or(viper.GetString("static.dir"), "./static")
		log.Info().Str("prefix", staticPrefix).Str("dir", staticDir).Msg("register static file server")
		rootMux.Handle(staticPrefix, http.StripPrefix(staticPrefix, http.FileServer(http.Dir(staticDir))))
	}

	// 2. API Gateway
	rootMux.Handle("/", gatewayHandler(gwmux))
	return rootMux
}

// updateFileHandle 上传文件
func (s *gameServer) updateFileHandle(w http.ResponseWriter, r *http.Request, params map[string]string) {
	w.Header().Add("Content-Type", "application/json")

	// 返回错误
	failResp := func(w http.ResponseWriter, code basepb.Code, msg string) {
		w.WriteHeader(http.StatusBadRequest)
		response := map[string]any{
			"code":    int32(code),
			"message": msg,
		}
		_ = json.NewEncoder(w).Encode(response)
	}

	err := r.ParseMultipartForm(32 << 20)
	if err != nil {
		log.Error().Err(err).Msg("parse form error")
		failResp(w, basepb.Code_CODE_ARGUMENT, "parse form error: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Error().Err(err).Msg("get file error")
		failResp(w, basepb.Code_CODE_ARGUMENT, "get file error: "+err.Error())
		return
	}
	defer file.Close()

	now := time.Now()
	rootDir := cmp.Or(viper.GetString("static.dir"), "./static")
	dirname := filepath.Join(rootDir, "upload", now.Format("2006-01-02"))
	if _, err := os.Stat(dirname); err != nil {
		_ = os.MkdirAll(dirname, 0755)
	}

	ext := filepath.Ext(header.Filename)
	filename := fmt.Sprintf("%d%d%s", time.Now().UnixNano()/1e6, 100+rand.Intn(899), ext)

	path := filepath.Join(dirname, filename)
	newFile, err := os.Create(path)
	if err != nil {
		log.Error().Err(err).Str("path", path).Msg("create file error")
		failResp(w, basepb.Code_CODE_INTERNAL_SERVER, "create file error: "+err.Error())
		return
	}
	defer newFile.Close()

	_, err = io.Copy(newFile, file)
	if err != nil {
		log.Error().Err(err).Str("path", path).Msg("write file error")
		failResp(w, basepb.Code_CODE_INTERNAL_SERVER, "write file error: "+err.Error())
		return
	}

	localName := filepath.Join("upload", now.Format("2006-01-02"), filename)
	response := map[string]any{
		"path": localName,
	}
	_ = json.NewEncoder(w).Encode(response)
}
