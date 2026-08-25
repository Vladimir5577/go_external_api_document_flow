package app

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go_external_api_document_flow/internal/config"

	"github.com/golang-jwt/jwt/v5"
)

// Сквозной тест: JWT → роль → запрос к don_snab с X-API-Key → форма ответа.
// Вместо внешнего сервиса поднимается заглушка, которая заодно проверяет,
// что именно микросервис ей отправил.

type upstreamCall struct {
	path   string
	query  string
	apiKey string
	auth   string
	body   string
}

func newStubUpstream(t *testing.T, calls *[]upstreamCall) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, upstreamCall{
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			apiKey: r.Header.Get("X-API-Key"),
			auth:   r.Header.Get("Authorization"),
			body:   string(body),
		})

		switch {
		case r.URL.Path == "/api/citizen-appeals" && r.Method == http.MethodGet:
			writeStub(w, http.StatusOK, `{"data":[{"id":7,"publicId":"OG-7","fio":"Иванов И.И.","appealType":"bulky_waste","city":"donetsk","status":"new","createdAt":"2026-08-12T10:00:00+03:00"}],"newAppealsCount":5,"pagination":{"page":1,"limit":20,"total":1,"pages":1}}`)

		case r.URL.Path == "/api/citizen-appeals/7" && r.Method == http.MethodGet:
			writeStub(w, http.StatusOK, `{"data":{"id":7,"publicId":"OG-7","fio":"Иванов И.И.","appealType":"bulky_waste","city":"donetsk","status":"done","message":"текст","replyTo":"email","createdAt":"2026-08-12T10:00:00+03:00","updatedAt":"2026-08-12T11:00:00+03:00","files":[{"id":3,"originalName":"скан.pdf","mimeType":"application/pdf","fileSize":1536,"url":"/internal/files/3"}]}}`)

		case r.URL.Path == "/api/citizen-appeals/7" && r.Method == http.MethodPatch:
			w.WriteHeader(http.StatusNoContent)

		case r.URL.Path == "/api/citizen-appeals/files/3":
			w.Header().Set("Content-Type", "application/pdf")
			w.Header().Set("Content-Disposition", `attachment; filename="скан.pdf"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("%PDF-1.4 stub"))

		case r.URL.Path == "/api/vacancies" && r.Method == http.MethodGet:
			writeStub(w, http.StatusOK, `{"data":[],"pagination":{"page":1,"limit":20,"total":0,"pages":0}}`)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func newStubVMAPI(t *testing.T, calls *[]upstreamCall) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, upstreamCall{
			path:  r.URL.Path,
			query: r.URL.RawQuery,
			auth:  r.Header.Get("Authorization"),
			body:  string(body),
		})

		if r.URL.Path != "/api/v1/health" && r.Header.Get("Authorization") != "Bearer vmapi-token" {
			writeStub(w, http.StatusUnauthorized, `{"error":"unauthorized"}`)
			return
		}

		switch {
		case r.URL.Path == "/api/v1/health" && r.Method == http.MethodGet:
			writeStub(w, http.StatusOK, `{"status":"ok","mailboxes":19}`)

		case r.URL.Path == "/api/v1/mailboxes" && r.Method == http.MethodGet:
			writeStub(w, http.StatusOK, `{"total_new":3,"mailboxes":[{"mailbox":"090","name":"Нерабочее время","new":3,"old":41},{"mailbox":"051","name":"Отдел обращения граждан","new":0,"old":2}]}`)

		case r.URL.Path == "/api/v1/mailboxes/090/messages" && r.Method == http.MethodGet:
			writeStub(w, http.StatusOK, `{"mailbox":"090","count":1,"truncated":false,"messages":[{"id":"1787591171-00000002","mailbox":"090","folder":"INBOX","caller_number":"+79495352139","caller_name":"","received_epoch":1787591171,"received_at":"2026-08-24T20:06:11+0300","duration_sec":9,"audio":{"format":"mp3","bitrate":"32k","size_bytes":39501,"base64":"SUQz"}}]}`)

		case r.URL.Path == "/api/v1/mailboxes/090/ack" && r.Method == http.MethodPost:
			writeStub(w, http.StatusOK, `{"mailbox":"090","moved":["1787591171-00000002"],"not_found":[]}`)

		case r.URL.Path == "/api/v1/mailboxes/090/messages/1787591171-00000002/audio" && r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "audio/mpeg")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("mp3-bytes"))

		default:
			writeStub(w, http.StatusNotFound, `{"error":"not found"}`)
		}
	}))
}

func writeStub(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

func newTestApp(t *testing.T, upstreamURL string) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	return newTestAppWithVMAPI(t, upstreamURL, "", "")
}

func newTestAppWithVMAPI(t *testing.T, upstreamURL, vmapiURL, vmapiToken string) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	t.Setenv("VMAPI_URL", vmapiURL)
	t.Setenv("VMAPI_TOKEN", vmapiToken)
	t.Setenv("VMAPI_TIMEOUT_SECONDS", "5")

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("не удалось сгенерировать ключ: %v", err)
	}

	keyPath := filepath.Join(t.TempDir(), "public.pem")
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("не удалось сериализовать публичный ключ: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), 0o600); err != nil {
		t.Fatalf("не удалось записать публичный ключ: %v", err)
	}

	regular, bold := systemFonts(t)

	application, err := NewApp(&config.Config{
		App: config.AppConfig{
			Env:      "test",
			Port:     "0",
			Location: time.UTC,
		},
		Auth: config.AuthConfig{
			JWTPublicKeyPath: keyPath,
		},
		DonSnab: config.DonSnabConfig{
			APIURL:  upstreamURL,
			APIKey:  "test-api-key",
			Timeout: 5 * time.Second,
		},
		PDF: config.PDFConfig{
			FontRegularPath: regular,
			FontBoldPath:    bold,
		},
	})
	if err != nil {
		t.Fatalf("не удалось создать приложение: %v", err)
	}

	server := httptest.NewServer(application.router)
	t.Cleanup(server.Close)

	return server, key
}

// systemFonts возвращает пути к DejaVu из пакета fonts-dejavu-core.
// Тот же шрифт ставится в образ сервиса, отдельной копии в репозитории нет.
func systemFonts(t *testing.T) (string, string) {
	t.Helper()

	regular := "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf"
	bold := "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"

	for _, path := range []string{regular, bold} {
		if _, err := os.Stat(path); err != nil {
			t.Skipf("нет системного шрифта %s — установите fonts-dejavu-core", path)
		}
	}

	return regular, bold
}

func token(t *testing.T, key *rsa.PrivateKey, roles ...string) string {
	t.Helper()

	claims := jwt.MapClaims{
		"id":       1,
		"username": "tester",
		"roles":    roles,
		"exp":      time.Now().Add(time.Hour).Unix(),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("не удалось подписать токен: %v", err)
	}

	return signed
}

func request(t *testing.T, server *httptest.Server, method, path, bearer, body string) *http.Response {
	t.Helper()

	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatalf("не удалось собрать запрос: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}

	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("запрос не прошёл: %v", err)
	}

	return resp
}

func TestRouterAuthAndRoles(t *testing.T) {
	var calls []upstreamCall
	upstream := newStubUpstream(t, &calls)
	defer upstream.Close()

	server, key := newTestApp(t, upstream.URL)

	t.Run("без токена 401", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/citizen-appeals", "", "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("статус = %d, ожидался 401", resp.StatusCode)
		}
	})

	t.Run("чужая роль 403", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/citizen-appeals", token(t, key, "ROLE_USER"), "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("статус = %d, ожидался 403", resp.StatusCode)
		}
	})

	t.Run("админ проходит по иерархии", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/vacancies", token(t, key, "ROLE_ADMIN"), "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("статус = %d, ожидался 200", resp.StatusCode)
		}
	})
}

func TestCitizenAppealFlow(t *testing.T) {
	var calls []upstreamCall
	upstream := newStubUpstream(t, &calls)
	defer upstream.Close()

	server, key := newTestApp(t, upstream.URL)
	bearer := token(t, key, "ROLE_CITIZEN_APPEAL")

	t.Run("список", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/citizen-appeals?status=new&page=1", bearer, "")
		defer resp.Body.Close()

		var payload map[string]any
		json.NewDecoder(resp.Body).Decode(&payload)

		items, _ := payload["items"].([]any)
		if len(items) != 1 {
			t.Fatalf("ожидался один элемент, получено %v", payload)
		}
		if items[0].(map[string]any)["appealTypeLabel"] != "КГО" {
			t.Errorf("лейбл типа обращения не проставлен: %v", items[0])
		}
		if payload["newAppealsCount"] != float64(5) {
			t.Errorf("newAppealsCount = %v", payload["newAppealsCount"])
		}

		last := calls[len(calls)-1]
		if last.apiKey != "test-api-key" {
			t.Errorf("X-API-Key не проброшен: %q", last.apiKey)
		}
		if !strings.Contains(last.query, "limit=20") || !strings.Contains(last.query, "status=new") {
			t.Errorf("фильтры собраны неверно: %q", last.query)
		}
	})

	t.Run("карточка подменяет url файла на свой прокси", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/citizen-appeals/7", bearer, "")
		defer resp.Body.Close()

		var payload map[string]any
		json.NewDecoder(resp.Body).Decode(&payload)

		data := payload["data"].(map[string]any)
		files := data["files"].([]any)
		file := files[0].(map[string]any)

		if file["url"] != "/spa/api/external-api/citizen-appeals/files/3" {
			t.Errorf("url файла = %v", file["url"])
		}
		if file["sizeLabel"] != "1.5 КБ" {
			t.Errorf("sizeLabel = %v", file["sizeLabel"])
		}
	})

	t.Run("patch отправляет только переданные поля и перечитывает карточку", func(t *testing.T) {
		calls = nil
		resp := request(t, server, http.MethodPatch, "/spa/api/external-api/citizen-appeals/7", bearer, `{"status":"done","adminComment":null}`)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("статус = %d, ожидался 200", resp.StatusCode)
		}
		if len(calls) != 2 {
			t.Fatalf("ожидались PATCH и повторный GET, получено %d вызовов", len(calls))
		}
		if calls[0].body != `{"status":"done"}` {
			t.Errorf("наверх ушло тело %q — adminComment: null не должен передаваться", calls[0].body)
		}
	})

	t.Run("выгрузка в pdf", func(t *testing.T) {
		calls = nil
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/citizen-appeals/7/pdf", bearer, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("статус = %d, ожидался 200", resp.StatusCode)
		}
		if resp.Header.Get("Content-Type") != "application/pdf" {
			t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
		}
		if disposition := resp.Header.Get("Content-Disposition"); !strings.Contains(disposition, "appeal-OG-7.pdf") {
			t.Errorf("Content-Disposition = %q", disposition)
		}
		// В выгрузке персональные данные — кэшировать её нельзя
		if cache := resp.Header.Get("Cache-Control"); cache != "no-store" {
			t.Errorf("Cache-Control = %q, ожидался no-store", cache)
		}

		body, _ := io.ReadAll(resp.Body)
		if !bytes.HasPrefix(body, []byte("%PDF")) {
			t.Errorf("тело не похоже на PDF: %q", body[:20])
		}

		// Карточка и вложение к ней — вложения тянутся для вклейки, а не проксируются
		if len(calls) != 2 || calls[1].path != "/api/citizen-appeals/files/3" {
			t.Errorf("ожидались запрос карточки и запрос вложения, получено %+v", calls)
		}
	})

	t.Run("файл проксируется потоком", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/citizen-appeals/files/3?download=1", bearer, "")
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if string(body) != "%PDF-1.4 stub" {
			t.Errorf("тело файла = %q", body)
		}
		if resp.Header.Get("Content-Type") != "application/pdf" {
			t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
		}
		if !strings.HasPrefix(resp.Header.Get("Content-Disposition"), "attachment") {
			t.Errorf("Content-Disposition = %q", resp.Header.Get("Content-Disposition"))
		}
		if last := calls[len(calls)-1]; last.query != "download=1" {
			t.Errorf("download не проброшен: %q", last.query)
		}
	})
}

func TestVoicemailFlow(t *testing.T) {
	var donsnabCalls []upstreamCall
	donsnab := newStubUpstream(t, &donsnabCalls)
	defer donsnab.Close()

	var vmapiCalls []upstreamCall
	vmapi := newStubVMAPI(t, &vmapiCalls)
	defer vmapi.Close()

	server, key := newTestAppWithVMAPI(t, donsnab.URL, vmapi.URL, "vmapi-token")
	bearer := token(t, key, "ROLE_CITIZEN_APPEAL")

	t.Run("чужая роль не проходит", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/mailboxes", token(t, key, "ROLE_USER"), "")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("статус = %d, ожидался 403", resp.StatusCode)
		}
	})

	t.Run("ящики нормализуются в camelCase", func(t *testing.T) {
		vmapiCalls = nil
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/mailboxes", bearer, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("статус = %d, ожидался 200", resp.StatusCode)
		}

		var payload map[string]any
		json.NewDecoder(resp.Body).Decode(&payload)

		if payload["totalNew"] != float64(3) {
			t.Errorf("totalNew = %v", payload["totalNew"])
		}

		items := payload["items"].([]any)
		first := items[0].(map[string]any)
		if first["newCount"] != float64(3) || first["oldCount"] != float64(41) {
			t.Errorf("счётчики ящика не нормализованы: %v", first)
		}
		if vmapiCalls[0].auth != "Bearer vmapi-token" {
			t.Errorf("Authorization в vmapi = %q", vmapiCalls[0].auth)
		}
	})

	t.Run("сообщения прокидывают query и добавляют audioUrl", func(t *testing.T) {
		vmapiCalls = nil
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/mailboxes/090/messages?audio=0&limit=10", bearer, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("статус = %d, ожидался 200", resp.StatusCode)
		}

		var payload map[string]any
		json.NewDecoder(resp.Body).Decode(&payload)

		items := payload["items"].([]any)
		msg := items[0].(map[string]any)
		if msg["callerNumber"] != "+79495352139" {
			t.Errorf("callerNumber = %v", msg["callerNumber"])
		}
		if msg["receivedAt"] != "2026-08-24T20:06:11+03:00" {
			t.Errorf("receivedAt = %v", msg["receivedAt"])
		}
		if msg["audioUrl"] != "/spa/api/external-api/voicemail/mailboxes/090/messages/1787591171-00000002/audio" {
			t.Errorf("audioUrl = %v", msg["audioUrl"])
		}
		if !strings.Contains(vmapiCalls[0].query, "audio=0") || !strings.Contains(vmapiCalls[0].query, "limit=10") {
			t.Errorf("query в vmapi = %q", vmapiCalls[0].query)
		}
	})

	t.Run("ack отправляет ids и нормализует notFound", func(t *testing.T) {
		vmapiCalls = nil
		resp := request(t, server, http.MethodPost, "/spa/api/external-api/voicemail/mailboxes/090/ack", bearer, `{"ids":["1787591171-00000002"]}`)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("статус = %d, ожидался 200", resp.StatusCode)
		}
		if vmapiCalls[0].body != `{"ids":["1787591171-00000002"]}` {
			t.Errorf("тело ack в vmapi = %q", vmapiCalls[0].body)
		}

		var payload map[string]any
		json.NewDecoder(resp.Body).Decode(&payload)
		if _, ok := payload["not_found"]; ok {
			t.Errorf("ответ не должен содержать snake_case not_found: %v", payload)
		}
		if _, ok := payload["notFound"]; !ok {
			t.Errorf("ответ должен содержать notFound: %v", payload)
		}
	})

	t.Run("audio проксируется потоком", func(t *testing.T) {
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/mailboxes/090/messages/1787591171-00000002/audio", bearer, "")
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		if string(body) != "mp3-bytes" {
			t.Errorf("тело audio = %q", body)
		}
		if resp.Header.Get("Content-Type") != "audio/mpeg" {
			t.Errorf("Content-Type = %q", resp.Header.Get("Content-Type"))
		}
		if cache := resp.Header.Get("Cache-Control"); cache != "no-store" {
			t.Errorf("Cache-Control = %q, ожидался no-store", cache)
		}
	})
}
