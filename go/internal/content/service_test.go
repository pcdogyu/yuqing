package content

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/stonedt-yuqing/go-jin10/internal/config"
	"github.com/stonedt-yuqing/go-jin10/internal/model"
	sqlitestore "github.com/stonedt-yuqing/go-jin10/internal/store/sqlite"
)

func TestArticleFilterFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/?page=2&page_size=50&keyword=%20alpha%20&source_type=flash&project_id=12&user_id=99&start=2026-05-01&end=2026-05-31", nil)

	filter := articleFilterFromRequest(req)

	if filter.Page != 2 || filter.PageSize != 50 {
		t.Fatalf("unexpected pagination filter: %+v", filter)
	}
	if filter.Keyword != "alpha" || filter.SourceType != "flash" {
		t.Fatalf("unexpected text filter: %+v", filter)
	}
	if filter.ProjectID != 12 || filter.UserID != 99 {
		t.Fatalf("unexpected id filter: %+v", filter)
	}
	if filter.Start != "2026-05-01" || filter.End != "2026-05-31" {
		t.Fatalf("unexpected range filter: %+v", filter)
	}

	req = httptest.NewRequest(http.MethodGet, "/?industry=finance&province=guangdong&city=shenzhen&sort=captured_at_desc&read=read&favorite=favorited", nil)
	filter = articleFilterFromRequest(req)
	if filter.Industry != "finance" || filter.Province != "guangdong" || filter.City != "shenzhen" {
		t.Fatalf("unexpected advanced filters: %+v", filter)
	}
	if filter.Sort != "captured_at_desc" || filter.Read != "read" || filter.Favorite != "favorited" {
		t.Fatalf("unexpected sort/state filters: %+v", filter)
	}
}

func TestDecodeJSON(t *testing.T) {
	t.Run("valid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"title":"hello"}`))
		recorder := httptest.NewRecorder()
		var payload struct {
			Title string `json:"title"`
		}

		if ok := decodeJSON(recorder, req, &payload); !ok {
			t.Fatal("expected decodeJSON to succeed")
		}
		if payload.Title != "hello" {
			t.Fatalf("expected decoded title, got %+v", payload)
		}
	})

	t.Run("invalid body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))
		recorder := httptest.NewRecorder()

		if ok := decodeJSON(recorder, req, &struct{}{}); ok {
			t.Fatal("expected decodeJSON to fail")
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
		}

		var payload map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if payload["message"] != "invalid body" {
			t.Fatalf("expected invalid body message, got %+v", payload)
		}
	})
}

func TestParseID(t *testing.T) {
	newRequest := func(id string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("id", id)
		return req.WithContext(contextWithRoute(req, rctx))
	}

	t.Run("valid id", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		id, ok := parseID(recorder, newRequest("42"), "id")
		if !ok || id != 42 {
			t.Fatalf("expected valid parsed id, got id=%d ok=%v", id, ok)
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		recorder := httptest.NewRecorder()
		id, ok := parseID(recorder, newRequest("bad"), "id")
		if ok || id != 0 {
			t.Fatalf("expected parse failure, got id=%d ok=%v", id, ok)
		}
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
		}
	})
}

func TestSummarizeTextAndNonEmpty(t *testing.T) {
	short := " short text "
	if got := summarizeText(short); got != "short text" {
		t.Fatalf("expected trimmed short text, got %q", got)
	}

	long := strings.Repeat("中", 141)
	got := summarizeText(long)
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected ellipsis for long text, got %q", got)
	}
	if len([]rune(got)) != 143 {
		t.Fatalf("expected 140 runes plus ellipsis, got %d runes", len([]rune(got)))
	}

	if got := nonEmpty("", "  ", " alpha ", "beta"); got != "alpha" {
		t.Fatalf("expected first non-empty trimmed value, got %q", got)
	}
}

func TestSearchSuggestionAndHotKeywordHandlers(t *testing.T) {
	store := newContentSearchTestStore(t)
	svc := NewService(config.Config{}, store)

	seed := []struct {
		userID int64
		word   string
	}{
		{42, "钢铁"},
		{42, "钢铁"},
		{42, "钢材"},
		{42, "能源"},
		{7, "钢铁"},
		{7, "科技"},
	}
	for _, item := range seed {
		if err := store.SaveSearchWord(context.Background(), item.userID, item.word); err != nil {
			t.Fatalf("SaveSearchWord error: %v", err)
		}
	}

	suggestionReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/suggestions?user_id=42&q=%E9%92%A2&limit=5", nil)
	suggestionRR := httptest.NewRecorder()
	svc.handleSearchSuggestions(suggestionRR, suggestionReq)
	if suggestionRR.Code != http.StatusOK {
		t.Fatalf("expected suggestions 200, got %d", suggestionRR.Code)
	}
	var suggestionEnvelope struct {
		Code int                    `json:"code"`
		Data []model.SearchWordStat `json:"data"`
	}
	if err := json.Unmarshal(suggestionRR.Body.Bytes(), &suggestionEnvelope); err != nil {
		t.Fatalf("unmarshal suggestions: %v", err)
	}
	if suggestionEnvelope.Code != http.StatusOK || len(suggestionEnvelope.Data) != 2 {
		t.Fatalf("unexpected suggestions payload: %+v", suggestionEnvelope)
	}
	if suggestionEnvelope.Data[0].SearchWord != "钢铁" || suggestionEnvelope.Data[0].WordCount != 2 {
		t.Fatalf("unexpected first suggestion: %+v", suggestionEnvelope.Data[0])
	}

	hotReq := httptest.NewRequest(http.MethodGet, "/api/v1/search/hot-keywords?limit=5", nil)
	hotRR := httptest.NewRecorder()
	svc.handleHotKeywords(hotRR, hotReq)
	if hotRR.Code != http.StatusOK {
		t.Fatalf("expected hot keywords 200, got %d", hotRR.Code)
	}
	var hotEnvelope struct {
		Code int                    `json:"code"`
		Data []model.SearchWordStat `json:"data"`
	}
	if err := json.Unmarshal(hotRR.Body.Bytes(), &hotEnvelope); err != nil {
		t.Fatalf("unmarshal hot keywords: %v", err)
	}
	if hotEnvelope.Code != http.StatusOK || len(hotEnvelope.Data) < 2 {
		t.Fatalf("unexpected hot keywords payload: %+v", hotEnvelope)
	}
	if hotEnvelope.Data[0].SearchWord != "钢铁" || hotEnvelope.Data[0].WordCount != 3 {
		t.Fatalf("unexpected hot keyword ranking: %+v", hotEnvelope.Data[0])
	}
}

func contextWithRoute(req *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
}

func newContentSearchTestStore(t *testing.T) *sqlitestore.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "content-search.db")
	store, err := sqlitestore.New(path)
	if err != nil {
		t.Fatalf("new sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}
