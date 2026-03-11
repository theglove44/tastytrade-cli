package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/theglove44/tastytrade-cli/internal/client"
	"github.com/theglove44/tastytrade-cli/internal/models"
)

// PageParam defines the query-parameter name and value encoding used by a
// particular endpoint family's pagination scheme.
//
// TastyTrade uses two schemes across its API surface:
//
//	OffsetPager  — ?page-offset=N  (0-based index, used by most list endpoints)
//	PagePager    — ?page=N         (1-based index, used by some transaction endpoints)
//
// Pass the appropriate constant to fetchAllPages.
type PageParam struct {
	// Key is the query parameter name, e.g. "page-offset" or "page".
	Key string
	// ZeroBased controls whether the first page index is 0 (true) or 1 (false).
	ZeroBased bool
}

var (
	// OffsetPager is the standard TastyTrade pagination scheme (most endpoints).
	// First page: no param (server default). Subsequent: ?page-offset=N (N>=1).
	OffsetPager = PageParam{Key: "page-offset", ZeroBased: true}

	// PagePager is used by endpoints that use 1-based page numbers.
	// First page: ?page=1. Subsequent: ?page=N.
	PagePager = PageParam{Key: "page", ZeroBased: false}
)

// nextURL returns the URL for the given page number.
// For OffsetPager page 0, the base URL is returned unmodified so the server
// uses its default (avoids a redundant ?page-offset=0 on the first request).
func (p PageParam) nextURL(baseURL string, page int) (string, error) {
	firstPage := 0
	if !p.ZeroBased {
		firstPage = 1
	}
	if page == firstPage && p.ZeroBased {
		// First page for offset-based: omit param entirely (server default = 0).
		return baseURL, nil
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("pagination: parse base URL %q: %w", baseURL, err)
	}
	q := u.Query()
	q.Set(p.Key, strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// fetchAllPages iterates a paginated TastyTrade items endpoint and returns
// the full aggregated slice of T.
//
// The pagination scheme is controlled by the pager parameter:
//   - Pass OffsetPager for endpoints using ?page-offset=N (the majority)
//   - Pass PagePager for endpoints using ?page=N
//
// All requests flow through client.Do — rate limiting, auth refresh, header
// injection, retry, and metrics apply automatically.
func fetchAllPages[T any](
	ctx context.Context,
	cl *client.Client,
	baseURL string,
	family string,
	pager PageParam,
	opts ...client.RequestOptions,
) ([]T, error) {
	var all []T

	firstPage := 0
	if !pager.ZeroBased {
		firstPage = 1
	}
	page := firstPage
	totalPages := firstPage + 1 // initialised to 1 page; updated from first response

	for page <= totalPages-(1-firstPage) {
		pageURL, err := pager.nextURL(baseURL, page)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
		if err != nil {
			return nil, fmt.Errorf("pagination: build request page %d: %w", page, err)
		}

		resp, err := cl.Do(ctx, req, family, opts...)
		if err != nil {
			return nil, fmt.Errorf("pagination: fetch page %d: %w", page, err)
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("pagination: HTTP %d on page %d: %s",
				resp.StatusCode, page, data)
		}

		var env models.ItemsEnvelope[T]
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("pagination: parse page %d: %w", page, err)
		}

		all = append(all, env.Data.Items...)

		// Update totalPages from response; guard against zero/missing Pagination.
		if p := env.Data.Pagination; p != nil && p.TotalPages > 0 {
			totalPages = p.TotalPages
		}

		page++
	}

	return all, nil
}
