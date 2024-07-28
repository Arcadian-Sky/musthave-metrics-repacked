package middleware

import (
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Arcadian-Sky/musthave-metrics/internal/server/flags"
)

func DecryptMiddleware(c flags.InitedFlags) func(http.Handler) http.Handler {
	return func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			privateKey, _ := c.GetCryptoKey()
			if privateKey != nil {
				// Читаем зашифрованные данные из тела запроса
				encryptedData, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "Ошибка при чтении данных", http.StatusBadRequest)
					return
				}
				defer r.Body.Close()

				// Расшифровываем данные
				decryptedData, err := decryptMessage(encryptedData, privateKey)
				if err != nil {
					http.Error(w, "Ошибка при расшифровке данных", http.StatusInternalServerError)
					return
				}

				// Подменяем тело запроса на расшифрованные данные
				r.Body = io.NopCloser(bytes.NewReader(decryptedData))
			}

			// Передаем управление следующему обработчику
			h.ServeHTTP(w, r)
		})
	}
}

func decryptMessage(encryptedMessage []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	return rsa.DecryptPKCS1v15(rand.Reader, privateKey, encryptedMessage)
}

// Добавьте поддержку gzip в код сервера и агента. Научите:
// Агент передавать данные в формате gzip.
// Сервер опционально принимать запросы в сжатом формате (при наличии соответствующего HTTP-заголовка Content-Encoding).
// Отдавать сжатый ответ клиенту, который поддерживает обработку сжатых ответов (с HTTP-заголовком Accept-Encoding).
// Функция сжатия должна работать для контента с типами application/json и text/html.

func GzipMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// по умолчанию устанавливаем оригинальный http.ResponseWriter как тот,
		// который будем передавать следующей функции
		ow := w

		// проверяем, что клиент умеет получать от сервера сжатые данные в формате gzip
		acceptContentType := r.Header.Get("Accept")
		acceptEncoding := r.Header.Get("Accept-Encoding")
		supportsGzip := strings.Contains(acceptEncoding, "gzip")
		if supportsGzip && (acceptContentType == "application/json" || acceptContentType == "html/text") {
			// оборачиваем оригинальный http.ResponseWriter новым с поддержкой сжатия
			cw := newCompressWriter(w)
			// меняем оригинальный http.ResponseWriter на новый
			ow = cw
			// не забываем отправить клиенту все сжатые данные после завершения middleware
			defer cw.Close()
		}

		// проверяем, что клиент отправил серверу сжатые данные в формате gzip
		contentEncoding := r.Header.Get("Content-Encoding")
		sendsGzip := strings.Contains(contentEncoding, "gzip")
		if sendsGzip {
			// оборачиваем тело запроса в io.Reader с поддержкой декомпрессии
			cr, err := newCompressReader(r.Body)
			fmt.Printf("err: %v\n", err)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			// меняем тело запроса на новое
			r.Body = cr
			defer cr.Close()
		}

		// передаём управление хендлеру
		h.ServeHTTP(ow, r)
	})
}

// compressWriter реализует интерфейс http.ResponseWriter и позволяет прозрачно для сервера
// сжимать передаваемые данные и выставлять правильные HTTP-заголовки
type compressWriter struct {
	w  http.ResponseWriter
	zw *gzip.Writer
}

func newCompressWriter(w http.ResponseWriter) *compressWriter {
	return &compressWriter{
		w:  w,
		zw: gzip.NewWriter(w),
	}
}

func (c *compressWriter) Header() http.Header {
	return c.w.Header()
}

func (c *compressWriter) Write(p []byte) (int, error) {
	return c.zw.Write(p)
}

func (c *compressWriter) WriteHeader(statusCode int) {
	// if statusCode < 300 {}
	c.w.Header().Set("Content-Encoding", "gzip")
	c.w.WriteHeader(statusCode)
}

// Close закрывает gzip.Writer и досылает все данные из буфера.
func (c *compressWriter) Close() error {
	return c.zw.Close()
}

// compressReader реализует интерфейс io.ReadCloser и позволяет прозрачно для сервера
// декомпрессировать получаемые от клиента данные
type compressReader struct {
	r  io.ReadCloser
	zr *gzip.Reader
}

func newCompressReader(r io.ReadCloser) (*compressReader, error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}

	return &compressReader{
		r:  r,
		zr: zr,
	}, nil
}

func (c compressReader) Read(p []byte) (n int, err error) {
	return c.zr.Read(p)
}

func (c *compressReader) Close() error {
	if err := c.r.Close(); err != nil {
		return err
	}
	return c.zr.Close()
}
