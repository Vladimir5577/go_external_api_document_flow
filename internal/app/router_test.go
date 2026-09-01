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
	path     string
	query    string
	apiKey   string
	auth     string
	body     string
	userID   string
	userName string
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

// newStubVoicemail — заглушка микросервиса голосовой почты. Шлюз в его ответ
// не заглядывает, поэтому заглушке достаточно записать, что до неё доехало.
func newStubVoicemail(t *testing.T, calls *[]upstreamCall) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, upstreamCall{
			path:     r.URL.Path,
			query:    r.URL.RawQuery,
			body:     string(body),
			userID:   r.Header.Get("X-User-Id"),
			userName: r.Header.Get("X-User-Name"),
		})

		writeStub(w, http.StatusOK, `{"items":[],"pagination":{"page":1,"limit":20,"total":0,"pages":0}}`)
	}))
}

func writeStub(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

func newTestApp(t *testing.T, upstreamURL string) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	return newTestAppWithVoicemail(t, upstreamURL, "")
}

func newTestAppWithVoicemail(t *testing.T, upstreamURL, voicemailURL string) (*httptest.Server, *rsa.PrivateKey) {
	t.Helper()

	t.Setenv("VOICEMAIL_SERVICE_URL", voicemailURL)

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

// Шлюз стал тонким: он проверяет роль, подписывает запрос личностью из JWT и
// передаёт всё остальное микросервису как есть. Что именно внутри — не его дело.
func TestVoicemailProxy(t *testing.T) {
	var donsnabCalls []upstreamCall
	donsnab := newStubUpstream(t, &donsnabCalls)
	defer donsnab.Close()

	var calls []upstreamCall
	voicemail := newStubVoicemail(t, &calls)
	defer voicemail.Close()

	server, key := newTestAppWithVoicemail(t, donsnab.URL, voicemail.URL)
	bearer := token(t, key, "ROLE_CITIZEN_APPEAL")

	t.Run("чужая роль не проходит", func(t *testing.T) {
		calls = nil
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/messages", token(t, key, "ROLE_USER"), "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("статус = %d, ожидался 403", resp.StatusCode)
		}
		if len(calls) != 0 {
			t.Errorf("запрос дошёл до микросервиса, хотя роли нет: %v", calls)
		}
	})

	t.Run("путь и query доезжают без изменений", func(t *testing.T) {
		calls = nil
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/messages?status=spam&page=2", bearer, "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("статус = %d, ожидался 200", resp.StatusCode)
		}
		if len(calls) != 1 {
			t.Fatalf("вызовов микросервиса %d, ожидался 1", len(calls))
		}
		if calls[0].path != "/spa/api/external-api/voicemail/messages" {
			t.Errorf("путь = %q, префикс резать не надо", calls[0].path)
		}
		if calls[0].query != "status=spam&page=2" {
			t.Errorf("query = %q", calls[0].query)
		}
	})

	t.Run("личность из JWT уезжает заголовками", func(t *testing.T) {
		calls = nil
		resp := request(t, server, http.MethodPatch, "/spa/api/external-api/voicemail/messages/3", bearer, `{"status":"done"}`)
		defer resp.Body.Close()

		if len(calls) != 1 {
			t.Fatalf("вызовов микросервиса %d, ожидался 1", len(calls))
		}
		if calls[0].userID != "1" {
			t.Errorf("X-User-Id = %q, ожидался клейм id из токена", calls[0].userID)
		}
		if calls[0].userName != "tester" {
			t.Errorf("X-User-Name = %q, ожидался клейм username", calls[0].userName)
		}
		if calls[0].body != `{"status":"done"}` {
			t.Errorf("тело = %q, должно доезжать нетронутым", calls[0].body)
		}
	})

	// Микросервис своей авторизации не имеет и верит этим заголовкам. Если бы
	// клиент мог их подставить, любой пользователь подписывал бы комментарии
	// чужим идентификатором.
	t.Run("заголовки клиента затираются", func(t *testing.T) {
		calls = nil

		req, err := http.NewRequest(http.MethodGet, server.URL+"/spa/api/external-api/voicemail/messages", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.Header.Set("X-User-Id", "999")
		req.Header.Set("X-User-Name", "director")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if len(calls) != 1 {
			t.Fatalf("вызовов микросервиса %d, ожидался 1", len(calls))
		}
		if calls[0].userID == "999" || calls[0].userName == "director" {
			t.Errorf("подставленные клиентом заголовки доехали: id=%q name=%q",
				calls[0].userID, calls[0].userName)
		}
	})

	t.Run("микросервис недоступен — 502", func(t *testing.T) {
		down := newStubVoicemail(t, &calls)
		down.Close()

		server, key := newTestAppWithVoicemail(t, donsnab.URL, down.URL)
		resp := request(t, server, http.MethodGet, "/spa/api/external-api/voicemail/messages",
			token(t, key, "ROLE_CITIZEN_APPEAL"), "")
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadGateway {
			t.Errorf("статус = %d, ожидался 502", resp.StatusCode)
		}
	})
}
